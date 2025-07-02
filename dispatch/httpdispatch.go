// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint: gosec
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/v2/logging" //nolint: staticcheck
	"github.com/xmidt-org/webpa-common/v2/xhttp"   //nolint: staticcheck
	"github.com/xmidt-org/wrp-go/v3"
)

// SenderConfig contains config to construct HTTPDispatcher, Transport, and Filter.
type SenderConfig struct {
	MaxWorkers            int
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	DeliveryInterval      time.Duration
	DeliveryRetries       int
	FilterQueueSize       int
}

// DispatcherSender contains config to construct a HTTPDispatcher.
type DispatcherSender struct {
	DeliveryInterval time.Duration
	DeliveryRetries  int
}

// HTTPDispatcher implements the dispatcher interface to send events through http.
type HTTPDispatcher struct {
	measures         Measures
	logger           log.Logger
	deliveryRetries  int
	deliveryInterval time.Duration
	sender           func(*http.Request) (*http.Response, error)
	mutex            sync.RWMutex
	httpConfig       model.HttpConfig
}

// NewHTTPDispatcher creates http dispatcher used to implement dispatcher interface.
func NewHTTPDispatcher(ds *DispatcherSender, httpConfig model.HttpConfig, sender *http.Client, logger log.Logger, measures Measures) (*HTTPDispatcher, error) {

	_, err := url.ParseRequestURI(httpConfig.URL)
	if err != nil {
		logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to parse HTTP URL.")
		return nil, err
	}

	if ds.DeliveryRetries > 10 {
		ds.DeliveryRetries = 10
	}

	if ds.DeliveryInterval > 1*time.Hour {
		ds.DeliveryInterval = 1 * time.Hour
	}

	dispatcher := HTTPDispatcher{
		httpConfig:       httpConfig,
		sender:           (sender).Do,
		logger:           logger,
		deliveryRetries:  ds.DeliveryRetries,
		deliveryInterval: ds.DeliveryInterval,
		measures:         measures,
	}

	return &dispatcher, nil
}

func (h *HTTPDispatcher) Start(context.Context) error {
	return nil
}

// Send uses the configured HTTP client to send a WRP message
// as a JSON.  The request also includes a signature created from
// hashing the norn secret against the wrp message.
func (h *HTTPDispatcher) Send(msg *wrp.Message) {
	url := h.httpConfig.URL
	defer func() {
		if r := recover(); nil != r {
			h.measures.DroppedPanicCounter.Add(1.0)
			h.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "goroutine send() panicked",
				"url", url, "panic", r)
		}
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
	resp, err := xhttp.RetryTransactor(retryOptions, h.sender)(req) //nolint: bodyclose
	code = "failure"
	if nil != err {
		h.measures.DroppedNetworkErrCounter.Add(1.0)
	} else {
		// Report Result for metrics
		code = strconv.Itoa(resp.StatusCode)

		// read until the response is complete before closing to allow
		// connection reuse
		if nil != resp.Body {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	h.measures.DeliveryCounter.With("url", url, "code", code, "event", event).Add(1.0)

}

// Update updates secret for http dispatcher for a norn.
func (h *HTTPDispatcher) Update(norn model.Norn) {

	h.mutex.Lock()
	if h.httpConfig.Secret != norn.Destination.HttpConfig.Secret {
		h.httpConfig.Secret = norn.Destination.HttpConfig.Secret
	}
	h.mutex.Unlock()

}
