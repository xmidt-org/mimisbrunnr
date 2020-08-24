/**
 * Copyright 2020 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package dispatch

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/v2"
)

type Filter struct {
	norn         model.Norn
	filterQueue  atomic.Value
	wg           sync.WaitGroup
	measures     *Measures
	mutex        sync.RWMutex
	dropUntil    time.Time
	maxWorkers   int
	workers      semaphore.Interface
	failureMsg   FailureMessage
	cutOffPeriod time.Duration
	logger       log.Logger
	sender       func(*http.Request) (*http.Response, error)
	queueSize    int
	dispatcher   D
}

type FilterConfig struct {
	QueueSize    int
	SenderConfig SenderConfig
}

type D interface {
	Start(context.Context) error
	Send(*wrp.Message)
	Update(norn model.Norn)
}

type FailureMessage struct {
	Text         string `json:"text"`
	CutOffPeriod string `json:"cut_off_period"`
}

const (
	defaultMinQueueSize = 5
	minMaxWorkers       = 5
)

// failureText is human readable text for the failure message
const FailureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

func NewFilter(fc FilterConfig, dispatcher D, norn model.Norn, sender *http.Client) (*Filter, error) {
	if norn.DeviceID == "" {
		return nil, fmt.Errorf("invalid deviceID")
	}
	filter := Filter{
		dispatcher: dispatcher,
		failureMsg: FailureMessage{
			Text:         FailureText,
			CutOffPeriod: fc.SenderConfig.CutOffPeriod.String(),
		},
		norn:   norn,
		sender: (sender).Do,
	}
	filter.filterQueue.Store(make(chan *wrp.Message, fc.QueueSize))
	return &filter, nil
}

func (f *Filter) Start(_ context.Context) error {
	f.wg.Add(1)
	go f.sendEvents()
	return nil

}

func (f *Filter) Stop(_ context.Context) error {
	close(f.filterQueue.Load().(chan *wrp.Message))
	f.mutex.Lock()
	f.measures.EventQueueDepthGauge.Set(0.0)
	f.mutex.Unlock()
	f.wg.Wait()
	return nil
}

func (f *Filter) Filter(deviceID string, event *wrp.Message) {
	if deviceID == f.norn.DeviceID {
		select {
		case f.filterQueue.Load().(chan *wrp.Message) <- event:
			f.measures.EventQueueDepthGauge.Add(1.0)

		default:
			f.queueOverflow()
			f.measures.DroppedQueueCount.Add(1.0)
		}
	}
}

// called if queue is filled
func (f *Filter) queueOverflow() {

	f.mutex.Lock()
	if time.Now().Before(f.dropUntil) {
		f.mutex.Unlock()
		return
	}
	f.dropUntil = time.Now().Add(f.cutOffPeriod)
	f.measures.DropUntilGauge.Set(float64(f.dropUntil.Unix()))
	secret := f.norn.Destination.HttpConfig.Secret
	failureMsg := f.failureMsg
	failureURL := f.norn.Destination.HttpConfig.FailureURL
	f.mutex.Unlock()

	var (
		errorLog = log.WithPrefix(f.logger, level.Key(), level.ErrorValue())
	)

	f.measures.CutOffCounter.Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	f.empty(f.measures.DroppedCutoffCounter)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		errorLog.Log(logging.MessageKey(), "Cut-off notification json.Marshal failed", "failureMessage", f.failureMsg,
			"for", f.norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
		return
	}

	// if no URL to send cut off notification to, do nothing
	if "" == failureURL {
		return
	}

	// Send a "you've been cut off" warning message
	payload := bytes.NewReader(msg)
	req, err := http.NewRequest("POST", failureURL, payload)
	if nil != err {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
			failureURL, "for", f.norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	if "" != secret {
		h := hmac.New(sha1.New, []byte(secret))
		h.Write(msg)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
		req.Header.Set("X-Codex-Signature", sig)
	}

	//  record content type, json.
	f.measures.ContentTypeCounter.With("content_type", "json").Add(1.0)
	resp, err := f.sender(req)
	if nil != err {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
			failureURL, "for", f.norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
		return
	}

	if nil == resp {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification, nil response",
			"notification", failureURL)
		return
	}

}

func (f *Filter) empty(droppedCounter metrics.Counter) {
	droppedMsgs := f.filterQueue.Load().(chan *wrp.Message)
	f.filterQueue.Store(make(chan *wrp.Message, f.queueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	f.measures.EventQueueDepthGauge.Set(0.0)
	return
}

func (f *Filter) sendEvents() {
Loop:
	for {
		defer f.wg.Done()
		queue := f.filterQueue.Load().(chan *wrp.Message)
		select {

		case en, ok := <-queue:
			if !ok {
				break Loop
			}
			f.measures.EventQueueDepthGauge.Add(-1.0)

			f.mutex.RLock()
			deliverUntil := time.Unix(0, f.norn.ExpiresAt)
			dropUntil := f.dropUntil
			f.mutex.RUnlock()

			now := time.Now()

			if now.Before(dropUntil) {
				f.measures.DroppedCutoffCounter.Add(1.0)
				continue
			}
			if now.After(deliverUntil) {
				f.measures.DroppedExpiredCounter.Add(1.0)
				continue
			}

			f.workers.Acquire()
			f.measures.WorkersCount.Add(1.0)
			go f.dispatcher.Send(en)
		}
		for i := 0; i < f.maxWorkers; i++ {
			f.workers.Acquire()
		}

		return
	}

}

func (f *Filter) Update(norn model.Norn) {
	f.mutex.Lock()
	if f.norn.ExpiresAt != norn.ExpiresAt {
		f.norn.ExpiresAt = norn.ExpiresAt
	}
	f.mutex.Unlock()
}
