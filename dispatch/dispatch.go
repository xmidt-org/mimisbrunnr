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
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/wrp-go/v2"
	// "github.com/xmidt-org/wrp-go/wrp"
)

const (
	defaultMinQueueSize = 5
	minMaxWorkers       = 5
)

type D interface {
	Start(context.Context) error
	Dispatch(deviceID string, msg *wrp.Message) error
	Update(norn model.Norn)
	Stop(context.Context) error
}

// failureText is human readable text for the failure message
const FailureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

func (d Dispatcher) Start(_ context.Context) error {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(d.Norn.Destination.AWSConfig.Sqs.Region),
		Credentials: credentials.NewStaticCredentials(d.Norn.Destination.AWSConfig.ID, d.Norn.Destination.AWSConfig.AccessKey, d.Norn.Destination.AWSConfig.SecretKey),
	})
	if err != nil {
		return err
	}
	d.SqsClient = sqs.New(sess)

	d.Wg.Add(1)
	go d.sendEvents()
	return nil

}

func (d Dispatcher) Dispatch(deviceID string, msg *wrp.Message) error {
	if deviceID == d.Norn.DeviceID {
		en := NewEventWithID(msg, deviceID)
		select {
		case d.DispatchQueue.Load().(chan *eventWithID) <- en:
			d.Measures.EventQueueDepthGauge.Add(1.0)
		default:
			d.queueOverflow()
			d.Measures.DroppedQueueCount.Add(1.0)
		}
	}
	return nil
}

func (d Dispatcher) sendEvents() {
	defer d.Wg.Done()
	queue := d.DispatchQueue.Load().(chan *eventWithID)
	select {

	case en, ok := <-queue:
		if !ok {
			break
		}
		d.Measures.EventQueueDepthGauge.Add(-1.0)

		d.Mutex.RLock()
		deliverUntil := time.Unix(0, d.Norn.ExpiresAt)
		dropUntil := d.DropUntil
		d.Mutex.RUnlock()

		now := time.Now()

		if now.Before(dropUntil) {
			d.Measures.DroppedCutoffCounter.Add(1.0)
		}
		if now.After(deliverUntil) {
			d.Measures.DroppedExpiredCounter.Add(1.0)
		}

		d.Workers.Acquire()
		d.Measures.WorkersCount.Add(1.0)
		go d.send(en.Event)
	}
	for i := 0; i < d.MaxWorkers; i++ {
		d.Workers.Acquire()
	}

	return

}

func (d *Dispatcher) empty(droppedCounter metrics.Counter) {
	droppedMsgs := d.DispatchQueue.Load().(chan *eventWithID)
	d.DispatchQueue.Store(make(chan *wrp.Message, d.QueueSize))
	droppedCounter.Add(float64(len(droppedMsgs)))
	d.Measures.EventQueueDepthGauge.Set(0.0)
	return
}

// called to deliver event
func (d Dispatcher) send(msg *wrp.Message) {
	defer func() {
		d.Wg.Done()
		if r := recover(); nil != r {
			d.Measures.DroppedPanicCounter.Add(1.0)
			d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "goroutine send() panicked",
				"id", d.Norn.DeviceID, "panic", r)
		}
		d.Workers.Release()
		d.Measures.WorkersCount.Add(-1.0)
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
		d.Measures.DroppedInvalidConfig.Add(1.0)
		d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to marshal event.")
		return
	}
	body := buffer.Bytes()
	payloadReader := bytes.NewReader(body)

	switch d.DestinationType {
	case SqsType:
		sqsParams := &sqs.SendMessageInput{
			MessageBody:  aws.String(string(body)),
			QueueUrl:     aws.String(d.Norn.Destination.AWSConfig.Sqs.QueueURL),
			DelaySeconds: aws.Int64(d.Norn.Destination.AWSConfig.Sqs.DelaySeconds),
		}
		_, err = d.SqsClient.SendMessage(sqsParams)
		if err != nil {
			d.Measures.DroppedNetworkErrCounter.Add(1.0)
			d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to send event to sqs.")
			return
		}
		url = d.Norn.Destination.AWSConfig.Sqs.QueueURL
		d.Measures.DeliveryCounter.With("url", url, "code", code, "event", event).Add(1.0)

	case HttpType:
		var (
			body []byte
		)
		req, err := http.NewRequest("POST", d.Norn.Destination.HttpConfig.URL, payloadReader)
		if nil != err {
			d.Measures.DroppedInvalidConfig.Add(1.0)
			d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Invalid URL",
				"url", d.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
			return
		}

		// Apply the secret
		if "" != d.Norn.Destination.HttpConfig.Secret {
			s := hmac.New(sha1.New, []byte(d.Norn.Destination.HttpConfig.Secret))
			s.Write(body)
			sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(s.Sum(nil)))
			req.Header.Set("X-Codex-Signature", sig)
		}

		// find the event "short name"
		event = msg.FindEventStringSubMatch()

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
		code = "failure"
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
		url = d.Norn.Destination.HttpConfig.URL
		d.Measures.DeliveryCounter.With("url", url, "code", code, "event", event).Add(1.0)

	default:
		d.Measures.DroppedNetworkErrCounter.Add(1.0)
	}

}

// called if queue is filled
func (d Dispatcher) queueOverflow() {

	d.Mutex.Lock()
	if time.Now().Before(d.DropUntil) {
		d.Mutex.Unlock()
		return
	}
	d.DropUntil = time.Now().Add(d.CutOffPeriod)
	d.Measures.DropUntilGauge.Set(float64(d.DropUntil.Unix()))
	secret := d.Norn.Destination.HttpConfig.Secret
	failureMsg := d.FailureMsg
	failureURL := d.Norn.Destination.HttpConfig.FailureURL
	d.Mutex.Unlock()

	var (
		errorLog = log.WithPrefix(d.Logger, level.Key(), level.ErrorValue())
	)

	d.Measures.CutOffCounter.Add(1.0)

	// We empty the queue but don't close the channel, because we're not
	// shutting down.
	d.empty(d.Measures.DroppedCutoffCounter)

	msg, err := json.Marshal(failureMsg)
	if nil != err {
		errorLog.Log(logging.MessageKey(), "Cut-off notification json.Marshal failed", "failureMessage", d.FailureMsg,
			"for", d.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
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
			failureURL, "for", d.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
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
	d.Measures.ContentTypeCounter.With("content_type", "json").Add(1.0)
	resp, err := d.Sender(req)
	if nil != err {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification", "notification",
			failureURL, "for", d.Norn.Destination.HttpConfig.URL, logging.ErrorKey(), err)
		return
	}

	if nil == resp {
		// Failure
		errorLog.Log(logging.MessageKey(), "Unable to send cut-off notification, nil response",
			"notification", failureURL)
		return
	}

}

func (d Dispatcher) Update(norn model.Norn) {
	if d.Norn.ExpiresAt != norn.ExpiresAt {
		d.Norn.ExpiresAt = norn.ExpiresAt
	}

	if reflect.DeepEqual(d.Norn.Destination.AWSConfig, norn.Destination.AWSConfig) == false {
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(norn.Destination.AWSConfig.Sqs.Region),
			Credentials: credentials.NewStaticCredentials(norn.Destination.AWSConfig.ID, norn.Destination.AWSConfig.AccessKey, norn.Destination.AWSConfig.SecretKey),
		})
		if err != nil {
			d.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to create new aws session.")
			return
		}
		d.SqsClient = sqs.New(sess)
		d.Norn.Destination.AWSConfig = norn.Destination.AWSConfig
	}

	if d.Norn.Destination.HttpConfig.Secret != norn.Destination.HttpConfig.Secret {
		d.Norn.Destination.HttpConfig.Secret = norn.Destination.HttpConfig.Secret
	}

}

func (d Dispatcher) Stop(context.Context) error {
	close(d.DispatchQueue.Load().(chan *eventWithID))

	d.Mutex.Lock()
	d.Measures.DeliverUntilGauge.Set(float64(d.Norn.ExpiresAt))
	d.Measures.EventQueueDepthGauge.Set(0.0)
	d.Mutex.Unlock()

	d.Wg.Wait()

	return nil
}
