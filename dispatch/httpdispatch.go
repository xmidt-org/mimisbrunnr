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

type SenderConfig struct {
	NumWorkersPerSender   int
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	CutOffPeriod          time.Duration
	DeliveryInterval      time.Duration
	DeliveryRetries       int
}

type HTTPDispatcher struct {
	measures         Measures
	workers          semaphore.Interface
	logger           log.Logger
	deliveryRetries  int
	deliveryInterval time.Duration
	sender           func(*http.Request) (*http.Response, error)
	mutex            sync.RWMutex
	httpConfig       model.HttpConfig
}

func NewHttpDispatcher(dc SenderConfig, httpConfig model.HttpConfig, sender *http.Client, logger log.Logger, measures Measures) (*HTTPDispatcher, error) {

	_, err := url.ParseRequestURI(httpConfig.URL)
	if err != nil {
		logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to parse HTTP URL.")
		return nil, err
	}

	if dc.DeliveryRetries > 10 {
		dc.DeliveryRetries = 10
	}

	if dc.DeliveryInterval > 1*time.Hour {
		dc.DeliveryInterval = 1 * time.Hour
	}

	dispatcher := HTTPDispatcher{
		httpConfig:       httpConfig,
		sender:           (sender).Do,
		logger:           logger,
		deliveryRetries:  dc.DeliveryRetries,
		deliveryInterval: dc.DeliveryInterval,
		measures:         measures,
	}

	return &dispatcher, nil
}

func (h *HTTPDispatcher) Start(_ context.Context) error {
	return nil

}

// called to deliver event
func (h *HTTPDispatcher) Send(msg *wrp.Message) {
	url := h.httpConfig.URL
	defer func() {
		if r := recover(); nil != r {
			h.measures.DroppedPanicCounter.Add(1.0)
			h.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "goroutine send() panicked",
				"url", url, "panic", r)
		}
		h.workers.Release()
		h.measures.WorkersCount.Add(-1.0)
	}()

	var (
		code  string
		event string
	)

	buffer := bytes.NewBuffer([]byte{})
	encoder := wrp.NewEncoder(buffer, wrp.JSON)
	err := encoder.Encode(msg)
	if err != nil {
		h.measures.DroppedInvalidConfig.Add(1.0)
		h.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to marshal event.")
		return
	}
	bodyPayload := buffer.Bytes()
	payloadReader := bytes.NewReader(bodyPayload)

	var (
		body []byte
	)
	req, err := http.NewRequest("POST", url, payloadReader)
	if nil != err {
		h.measures.DroppedInvalidConfig.Add(1.0)
		h.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Invalid URL",
			"url", url, logging.ErrorKey(), err)
		return
	}

	// Apply the secret
	if "" != h.httpConfig.Secret {
		s := hmac.New(sha1.New, []byte(h.httpConfig.Secret))
		s.Write(body)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(s.Sum(nil)))
		req.Header.Set("X-Codex-Signature", sig)
	}

	// find the event "short name"
	event = msg.FindEventStringSubMatch()

	retryOptions := xhttp.RetryOptions{
		Logger:      h.logger,
		Retries:     h.deliveryRetries,
		Interval:    h.deliveryInterval,
		ShouldRetry: func(error) bool { return true },
		ShouldRetryStatus: func(code int) bool {
			return code < 200 || code > 299
		},
	}
	resp, err := xhttp.RetryTransactor(retryOptions, h.sender)(req)
	code = "failure"
	if nil != err {
		h.measures.DroppedNetworkErrCounter.Add(1.0)
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
	h.measures.DeliveryCounter.With("url", url, "code", code, "event", event).Add(1.0)

}

func (h *HTTPDispatcher) Update(norn model.Norn) {

	h.mutex.Lock()
	if h.httpConfig.Secret != norn.Destination.HttpConfig.Secret {
		h.httpConfig.Secret = norn.Destination.HttpConfig.Secret
	}
	h.mutex.Unlock()

}
