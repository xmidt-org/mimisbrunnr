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

type F interface {
	Start(context.Context) error
	Filter(event *wrp.Message)
	Stop(context.Context) error
}

type Filter struct {
	DeviceId     string
	Norn         model.Norn
	FilterQueue  atomic.Value
	Wg           sync.WaitGroup
	Measures     *Measures
	Mutex        sync.RWMutex
	DropUntil    time.Time
	MaxWorkers   int
	Workers      semaphore.Interface
	FailureMsg   FailureMessage
	CutOffPeriod time.Duration
	Logger       log.Logger
	Sender       func(*http.Request) (*http.Response, error)
	QueueSize    int
	Dispatcher   D
}

type FilterConfig struct {
	QueueSize    int
	SenderConfig SenderConfig
}

type FilterDispatcher struct {
	Norn       model.Norn
	Dispatcher D
	Filter     F
}

func NewFilter(fc FilterConfig, deviceID string, dispatcher D, norn model.Norn, transport http.RoundTripper) (F, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("invalid deviceID")
	}
	filter := Filter{
		DeviceId:   deviceID,
		Dispatcher: dispatcher,
		FailureMsg: FailureMessage{
			Text:         FailureText,
			CutOffPeriod: fc.SenderConfig.CutOffPeriod.String(),
		},
		Norn: norn,
		Sender: (&http.Client{
			Transport: transport,
		}).Do,
	}
	filter.FilterQueue.Store(make(chan *wrp.Message, fc.QueueSize))
	return &filter, nil
}

func (f Filter) Start(_ context.Context) error {
	f.Wg.Add(1)
	go f.sendEvents()
	return nil

}

func (f Filter) Stop(_ context.Context) error {
	close(f.FilterQueue.Load().(chan *wrp.Message))
	f.Mutex.Lock()
	f.Measures.EventQueueDepthGauge.Set(0.0)
	f.Mutex.Unlock()
	f.Wg.Wait()
	return nil
}

func (f *Filter) Filter(event *wrp.Message) {
	if f.DeviceId == f.Norn.DeviceID {
		select {
		case f.FilterQueue.Load().(chan *wrp.Message) <- event:
			f.Measures.EventQueueDepthGauge.Add(1.0)

		default:
			f.queueOverflow()
			f.Measures.DroppedQueueCount.Add(1.0)
		}
	}
}

// called if queue is filled
func (f Filter) queueOverflow() {

	f.Mutex.Lock()
	if time.Now().Before(f.DropUntil) {
		f.Mutex.Unlock()
		return
	}
	f.DropUntil = time.Now().Add(f.CutOffPeriod)
	f.Measures.DropUntilGauge.Set(float64(f.DropUntil.Unix()))
	secret := f.Norn.Destination.HttpConfig.Secret
	failureMsg := f.FailureMsg
	failureURL := f.Norn.Destination.HttpConfig.FailureURL
	f.Mutex.Unlock()

	var (
		errorLog = log.WithPrefix(f.Logger, level.Key(), level.ErrorValue())
	)

	f.Measures.CutOffCounter.Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	f.empty(f.Measures.DroppedCutoffCounter)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		errorLog.Log(logging.MessageKey(), "Cut-off notification json.Marshal failed", "failureMessage", f.FailureMsg,
			"for", f.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
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
			failureURL, "for", f.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
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
	f.Measures.ContentTypeCounter.With("content_type", "json").Add(1.0)
	resp, err := f.Sender(req)
	if nil != err {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
			failureURL, "for", f.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
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
	droppedMsgs := f.FilterQueue.Load().(chan *wrp.Message)
	f.FilterQueue.Store(make(chan *wrp.Message, f.QueueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	f.Measures.EventQueueDepthGauge.Set(0.0)
	return
}

func (f Filter) sendEvents() {
	defer f.Wg.Done()
	queue := f.FilterQueue.Load().(chan *wrp.Message)
	select {

	case en, ok := <-queue:
		if !ok {
			break
		}
		f.Measures.EventQueueDepthGauge.Add(-1.0)

		f.Mutex.RLock()
		deliverUntil := time.Unix(0, f.Norn.ExpiresAt)
		dropUntil := f.DropUntil
		f.Mutex.RUnlock()

		now := time.Now()

		if now.Before(dropUntil) {
			f.Measures.DroppedCutoffCounter.Add(1.0)
		}
		if now.After(deliverUntil) {
			f.Measures.DroppedExpiredCounter.Add(1.0)
		}

		f.Workers.Acquire()
		f.Measures.WorkersCount.Add(1.0)
		go f.Dispatcher.Send(en)
	}
	for i := 0; i < f.MaxWorkers; i++ {
		f.Workers.Acquire()
	}

	return

}
