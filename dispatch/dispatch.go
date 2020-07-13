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
	"io"
	"io/ioutil"
	"strconv"
	"time"

	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/wrp-go/v2"
	"github.com/xmidt-org/wrp-go/v2/wrphttp"
)

const (
	defaultMinQueueSize = 5
	minMaxWorkers       = 5
)

type D interface {
	Start() func(context.Context) error
	Dispatch() error
	Update(norn model.Norn)
	Stop() func(context.Context) error
}

// failureText is human readable text for the failure message
const FailureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

func (d Dispatcher) Start() func(context.Context) error {
	return func(ctx context.Context) error {
		go d.Dispatch()
		return nil
	}
}

func (d Dispatcher) Dispatch() error {
	defer d.Wg.Wait()
	queue := d.DispatchQueue.Load().(chan *EventNorn)
	select {

	case en, ok := <-queue:
		if !ok {
			break
		}
		d.Measures.EventQueueDepthGauge.Add(-1.0)

		d.Mutex.RLock()
		deliverUntil := d.DeliverUntil
		dropUntil := d.DropUntil
		d.Mutex.RUnlock()

		now := time.Now()

		if now.Before(dropUntil) {
			d.Measures.DroppedCutoffCounter.Add(1.0)
		}
		if now.After(deliverUntil) {
			d.Measures.DroppedExpiredCounter.Add(1.0)
		}

		if en.DeviceID == en.Norn.DeviceID {
			d.Workers.Acquire()
			d.Measures.WorkersCount.Add(1.0)
			go d.send(en.Event, en.Norn.Destination.Type, en.Norn.Destination.HttpConfig.AcceptType, en.Norn)
		}
	}
	for i := 0; i < d.MaxWorkers; i++ {
		d.Workers.Acquire()
	}

	return nil

}

func (d *Dispatcher) Empty(droppedCounter metrics.Counter) {
	droppedMsgs := d.DispatchQueue.Load().(chan *EventNorn)
	d.DispatchQueue.Store(make(chan *wrp.Message, d.QueueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	d.Measures.EventQueueDepthGauge.Set(0.0)
	return
}

// called to deliver event
func (d Dispatcher) send(msg *wrp.Message, destType string, acceptType string, norn model.Norn) {
	switch destType {
	case "sqs":
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(norn.Destination.AWSConfig.Sqs.Region),
			Credentials: credentials.NewStaticCredentials(norn.Destination.AWSConfig.ID, norn.Destination.AWSConfig.AccessKey, norn.Destination.AWSConfig.SecretKey),
		})
		if err != nil {
			d.Measures.DroppedInvalidConfig.Add(1.0)
			d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to create new aws session.")
			return
		}
		sqsClient := sqs.New(sess)

		// Send message
		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			d.Measures.DroppedInvalidConfig.Add(1.0)
			d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to marshal event.")
			return
		}
		sqsParams := &sqs.SendMessageInput{
			MessageBody:  aws.String(string(jsonMsg)),
			QueueUrl:     aws.String(norn.Destination.AWSConfig.Sqs.QueueURL),
			DelaySeconds: aws.Int64(3),
		}
		_, err = sqsClient.SendMessage(sqsParams)
		if err != nil {
			d.Measures.DroppedNetworkErrCounter.Add(1.0)
			d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to send event to sqs.")
			return
		}
		d.Measures.DeliveryCounter.With("url", norn.Destination.AWSConfig.Sqs.QueueURL)

	case "http":
		var (
			body []byte
		)
		contentType := msg.ContentType
		switch acceptType {
		case "wrp", "application/msgpack", "application/wrp":
			contentType = "application/msgpack"
			buffer := bytes.NewBuffer([]byte{})
			encoder := wrp.NewEncoder(buffer, wrp.Msgpack)
			encoder.Encode(msg)
			body = buffer.Bytes()
		}
		payloadReader := bytes.NewReader(body)

		req, err := http.NewRequest("POST", norn.Destination.HttpConfig.URL, payloadReader)
		if nil != err {
			d.Measures.DroppedInvalidConfig.Add(1.0)
			d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Invalid URL",
				"url", norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
			return
		}
		req.Header.Set("Content-Type", contentType)

		// Add x-Midt-* headers
		wrphttp.AddMessageHeaders(req.Header, msg)

		// Provide the old headers for now
		req.Header.Set("X-Webpa-Event", strings.TrimPrefix(msg.Destination, "event:"))
		req.Header.Set("X-Webpa-Transaction-Id", msg.TransactionUUID)

		// Add the device id without the trailing service
		id, _ := device.ParseID(msg.Source)
		req.Header.Set("X-Webpa-Device-Id", string(id))
		req.Header.Set("X-Webpa-Device-Name", string(id))

		// Apply the secret
		if "" != norn.Destination.HttpConfig.Secret {
			s := hmac.New(sha1.New, []byte(norn.Destination.HttpConfig.Secret))
			s.Write(body)
			sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(s.Sum(nil)))
			req.Header.Set("X-Webpa-Signature", sig)
		}

		// find the event "short name"
		event := msg.FindEventStringSubMatch()

		retryOptions := xhttp.RetryOptions{
			Logger:      d.Logger,
			Retries:     d.DeliveryRetries,
			Interval:    d.DeliveryInterval,
			ShouldRetry: func(error) bool { return true },
			ShouldRetryStatus: func(code int) bool {
				return code < 200 || code > 299
			},
		}
		resp, err := xhttp.RetryTransactor(retryOptions, d.Sender)(req)
		code := "failure"
		if nil != err {
			d.Measures.DroppedNetworkErrCounter.Add(1.0)
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
		d.Measures.DeliveryCounter.With("url", norn.Destination.HttpConfig.URL, "code", code, "event", event).Add(1.0)
	}

}

// called if queue is filled
func (d Dispatcher) queueOverflow(en *EventNorn) {

	d.Mutex.Lock()
	if time.Now().Before(d.DropUntil) {
		d.Mutex.Unlock()
		return
	}
	d.DropUntil = time.Now().Add(d.CutOffPeriod)
	d.Measures.DropUntilGauge.Set(float64(d.DropUntil.Unix()))
	secret := en.Norn.Destination.HttpConfig.Secret
	failureMsg := d.FailureMsg
	failureURL := en.Norn.Destination.HttpConfig.FailureURL
	d.Mutex.Unlock()

	var (
		errorLog = log.WithPrefix(d.Logger, level.Key(), level.ErrorValue())
	)

	d.Measures.CutOffCounter.Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	d.Empty(d.Measures.DroppedCutoffCounter)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		errorLog.Log(logging.MessageKey(), "Cut-off notification json.Marshal failed", "failureMessage", d.FailureMsg,
			"for", en.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
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
			failureURL, "for", en.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	if "" != secret {
		h := hmac.New(sha1.New, []byte(secret))
		h.Write(msg)
		sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
		req.Header.Set("X-Webpa-Signature", sig)
	}

	//  record content type, json.
	d.Measures.ContentTypeCounter.With("content_type", "json").Add(1.0)
	resp, err := d.Sender(req)
	if nil != err {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
			failureURL, "for", en.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
		return
	}

	if nil == resp {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification, nil response",
			"notification", failureURL)
		return
	}

}

// update TTL for norn
func (d Dispatcher) Update(norn model.Norn) {

}

func (d Dispatcher) Stop() func(context.Context) error {
	return func(ctx context.Context) error {
		close(d.DispatchQueue.Load().(chan *EventNorn))
		d.Wg.Wait()

		d.Mutex.Lock()
		d.DeliverUntil = time.Time{}
		d.Measures.DeliverUntilGauge.Set(float64(d.DeliverUntil.Unix()))
		d.Measures.EventQueueDepthGauge.Set(0.0)
		d.Mutex.Unlock()

		close(d.DispatchQueue.Load().(chan *EventNorn))
		return nil
	}
}
