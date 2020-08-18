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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/wrp-go/v2"
)

type DispatcherConfig struct {
	QueueSize    int
	NumWorkers   int
	SenderConfig SenderConfig
	Norn         model.Norn
}

type SenderConfig struct {
	NumWorkersPerSender   int
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	CutOffPeriod          time.Duration
	DeliveryInterval      time.Duration
}

type HttpDispatcher struct {
	Measures         *Measures
	Workers          semaphore.Interface
	Logger           log.Logger
	DeliveryRetries  int
	DeliveryInterval time.Duration
	Sender           func(*http.Request) (*http.Response, error)
	Mutex            sync.RWMutex
	Wg               sync.WaitGroup
	Norn             model.Norn
}

func NewHttpDispatcher(dc DispatcherConfig, norn model.Norn, sender *http.Client) (D, error) {
	if dc.QueueSize < defaultMinQueueSize {
		dc.QueueSize = defaultMinQueueSize
	}

	if dc.NumWorkers < minMaxWorkers {
		dc.NumWorkers = minMaxWorkers
	}

	dispatcher := HttpDispatcher{
		Norn:   norn,
		Sender: (sender).Do,
	}

	_, err := url.ParseRequestURI(norn.Destination.HttpConfig.URL)
	if err != nil {
		return dispatcher, nil
	}

	return dispatcher, nil
}

func (h HttpDispatcher) Start(_ context.Context) error {
	return nil

}

// called to deliver event
func (h HttpDispatcher) Send(msg *wrp.Message) {
	defer func() {
		h.Wg.Done()
		if r := recover(); nil != r {
			h.Measures.DroppedPanicCounter.Add(1.0)
			h.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "goroutine send() panicked",
				"id", h.Norn.DeviceID, "panic", r)
		}
		h.Workers.Release()
		h.Measures.WorkersCount.Add(-1.0)
	}()

	var (
		url   string
		code  string
		event string
	)

	buffer := bytes.NewBuffer([]byte{})
	encoder := wrp.NewEncoder(buffer, wrp.JSON)
	err := encoder.Encode(msg)
	if err != nil {
		h.Measures.DroppedInvalidConfig.Add(1.0)
		h.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to marshal event.")
		return
	}
	bodyPayload := buffer.Bytes()
	payloadReader := bytes.NewReader(bodyPayload)

	var (
		body []byte
	)
	req, err := http.NewRequest("POST", h.Norn.Destination.HttpConfig.URL, payloadReader)
	if nil != err {
		h.Measures.DroppedInvalidConfig.Add(1.0)
		h.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Invalid URL",
			"url", h.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
		return
	}

	// Apply the secret
	if "" != h.Norn.Destination.HttpConfig.Secret {
		s := hmac.New(sha1.New, []byte(h.Norn.Destination.HttpConfig.Secret))
		s.Write(body)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(s.Sum(nil)))
		req.Header.Set("X-Codex-Signature", sig)
	}

	// find the event "short name"
	event = msg.FindEventStringSubMatch()

	retryOptions := xhttp.RetryOptions{
		Logger:      h.Logger,
		Retries:     h.DeliveryRetries,
		Interval:    h.DeliveryInterval,
		ShouldRetry: func(error) bool { return true },
		ShouldRetryStatus: func(code int) bool {
			return code < 200 || code > 299
		},
	}
	resp, err := xhttp.RetryTransactor(retryOptions, h.Sender)(req)
	code = "failure"
	if nil != err {
		h.Measures.DroppedNetworkErrCounter.Add(1.0)
	} else {
		// Report Result for metrics
		code = strconv.Itoa(resp.StatusCode)

		// read until the response is complete before closing to allow
		// connection reuse
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	url = h.Norn.Destination.HttpConfig.URL
	h.Measures.DeliveryCounter.With("url", url, "code", code, "event", event).Add(1.0)

}

func (h HttpDispatcher) Update(norn model.Norn) {

	if h.Norn.Destination.HttpConfig.Secret != norn.Destination.HttpConfig.Secret {
		h.Norn.Destination.HttpConfig.Secret = norn.Destination.HttpConfig.Secret
	}

}
