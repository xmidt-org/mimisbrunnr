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
	"github.com/xmidt-org/mimisbrunnr/eventParser"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/v2"
)

// Filter is used to implement Filterer interface
type Filter struct {
	norn         model.Norn
	filterQueue  atomic.Value
	wg           sync.WaitGroup
	measures     *Measures
	mutex        sync.RWMutex
	dropUntil    time.Time
	maxWorkers   int
	workers      semaphore.Interface
	failureMsg   failureMessage
	cutOffPeriod time.Duration
	logger       log.Logger
	sender       func(*http.Request) (*http.Response, error)
	queueSize    int
	dispatcher   D
}

// FilterConfig contains the configuration needed to construct a Filter.
type FilterConfig struct {
	QueueSize int
}

// FilterSender contains config to contruct Filter
type FilterSender struct {
	QueueSize  int
	NumWorkers int
}

// D is dispatcher interface that abstracts away the exact delivery mechanism (ie HTTP or SQS).
type D interface {
	// Start sets up any initial sessions or connections needed in order to send events.
	Start(context.Context) error

	// Send delivers the message.  Sending is done concurrently, so no error is returned.
	Send(*wrp.Message)

	// Update is called to make sure the dispatcher keeps up to date with the norn config.
	Update(norn model.Norn)
}

type failureMessage struct {
	Text string `json:"text"`
}

const (
	defaultMinQueueSize = 100
	minWorkers          = 5
)

// failureText is human readable text for the failure message
const failureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

// NewFilter validates the config and creates a new Filter.
func NewFilter(fs *FilterSender, dispatcher D, norn model.Norn, sender *http.Client, measures Measures) (*Filter, error) {
	if norn.DeviceID == "" {
		return nil, fmt.Errorf("invalid deviceID")
	}

	if fs.QueueSize < eventParser.DefaultMinQueueSize {
		fs.QueueSize = eventParser.DefaultMinQueueSize
	}

	if fs.NumWorkers < minWorkers {
		fs.NumWorkers = minWorkers
	}

	filter := Filter{
		dispatcher: dispatcher,
		failureMsg: failureMessage{
			Text: failureText,
		},
		norn:     norn,
		measures: &measures,
		sender:   (sender).Do,
	}
	filter.filterQueue.Store(make(chan *wrp.Message, fs.QueueSize))
	return &filter, nil
}

// Start begins pulling from queue to deliver events.
func (f *Filter) Start(context.Context) error {
	f.wg.Add(1)
	go f.sendEvents()
	return nil

}

// Stop closes the queue and resets its metric.
func (f *Filter) Stop(context.Context) error {
	close(f.filterQueue.Load().(chan *wrp.Message))
	f.mutex.Lock()
	f.measures.EventQueueDepthGauge.Set(0.0)
	f.mutex.Unlock()
	f.wg.Wait()
	return nil
}

// Filter decides if an event should be sent by checking if the
// event's device ID matches the device ID of the norn.  If they match,
// the event is queued to be dispatched.  If not, the event is dropped.
// If the queue is full when queueing the event, the event is dropped, the
// queue is emptied and the norn consumer is cut off.
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

// queueOverflow is called when the queue is full and can no longer enqueue
// events. The norn is then cut off for a certain period of time and sends a
// failure message to the provided failure URL for an HTTP event delivery mechanism.
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
			go func() {
				f.dispatcher.Send(en)
				f.workers.Release()
				f.measures.WorkersCount.Add(-1.0)
			}()
		}
		for i := 0; i < f.maxWorkers; i++ {
			f.workers.Acquire()
		}

		return
	}

}

// Update will update the time a norn expires.
func (f *Filter) Update(norn model.Norn) {
	f.mutex.Lock()
	if f.norn.ExpiresAt != norn.ExpiresAt {
		f.norn.ExpiresAt = norn.ExpiresAt
	}
	f.mutex.Unlock()
}
