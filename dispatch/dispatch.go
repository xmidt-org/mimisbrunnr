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
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/wrp-go/v2"
	"github.com/xmidt-org/wrp-go/v2/wrphttp"
)

const (
	defaultMinQueueSize = 5
	minMaxWorkers       = 5
)

type D interface {
	Queue(en *EventNorn)
	Dispatch() error
	Update(norn model.Norn)
	Stop()
}

type DispatcherConfig struct {
	// info needed to create New Dispatchers
	// add workers to config
	QueueSize    int
	MaxWorkers   int
	SenderConfig SenderConfig
}

type SenderConfig struct {
	NumWorkersPerSender   int
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
}

type Dispatcher struct {
	// DispatchQueue chan *EventNorn
	DispatchQueue    atomic.Value
	Measures         *Measures
	Workers          semaphore.Interface
	MaxWorkers       int
	Logger           log.Logger
	DeliveryRetries  int
	DeliveryInterval time.Duration
	QueueSize        int
	Sender           func(*http.Request) (*http.Response, error)
}

type EventNorn struct {
	Event    *wrp.Message
	Norn     model.Norn
	DeviceID string
}

func NewDispatcher(dc DispatcherConfig) (D, error) {
	if dc.QueueSize < defaultMinQueueSize {
		dc.QueueSize = defaultMinQueueSize
	}

	if dc.MaxWorkers < minMaxWorkers {
		dc.MaxWorkers = minMaxWorkers
	}

	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		MaxIdleConnsPerHost:   dc.SenderConfig.NumWorkersPerSender,
		ResponseHeaderTimeout: dc.SenderConfig.ResponseHeaderTimeout,
		IdleConnTimeout:       dc.SenderConfig.IdleConnTimeout,
	}

	dispatcher := Dispatcher{
		MaxWorkers: dc.MaxWorkers,
		QueueSize:  dc.QueueSize,
		Sender: (&http.Client{
			Transport: tr,
		}).Do,
	}

	dispatcher.DispatchQueue.Store(make(chan *EventNorn, dc.QueueSize))

	return dispatcher, nil
}

func NewEventNorn(event *wrp.Message, norn model.Norn, deviceID string) *EventNorn {
	return &EventNorn{
		Event:    event,
		Norn:     norn,
		DeviceID: deviceID,
	}
}

func (d Dispatcher) Queue(en *EventNorn) {
	select {
	case d.DispatchQueue.Load().(chan *EventNorn) <- en:
		d.Measures.EventQueueDepthGauge.Add(1.0)
	default:
		d.queueOverflow()
		d.Measures.DroppedQueueCount.Add(1.0)
	}
}

func (d Dispatcher) Dispatch() error {

	queue := d.DispatchQueue.Load().(chan *EventNorn)
	select {

	case en, ok := <-queue:
		if !ok {
			break
		}
		d.Measures.EventQueueDepthGauge.Add(-1.0)
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

func (d *Dispatcher) Empty() {
	// droppedMsgs := d.DispatchQueue.Load().(chan *EventNorn)
	d.DispatchQueue.Store(make(chan *wrp.Message, d.QueueSize))
	// add metric for dropped events
	// add metric for queueDepth and set to 0
	return
}

// called to deliver event
func (d Dispatcher) send(msg *wrp.Message, destType string, acceptType string, norn model.Norn) {
	switch destType {
	case "sqs":
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(norn.Destination.AWSConfig.Sqs.Region),
			Credentials: credentials.NewStaticCredentials(norn.Destination.AWSConfig.ID, norn.Destination.AWSConfig.AccessKey, norn.Destination.AWSConfig.SecretKey),
			// add delay and/or maxretries
		})
		if err != nil {
			// return aws_err
			// add logging
		}
		sqsClient := sqs.New(sess)

		// Send message
		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			// add logging
		}
		sqsParams := &sqs.SendMessageInput{
			MessageBody:  aws.String(string(jsonMsg)),
			QueueUrl:     aws.String(norn.Destination.AWSConfig.Sqs.QueueURL),
			DelaySeconds: aws.Int64(3),
		}
		_, err = sqsClient.SendMessage(sqsParams)
		// use resp from SendMessage to use for logging
		if err != nil {
			// add logging
		}
		// check resp and log
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
			// add metrics and logging for a dropped message
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
		// code := "failure"
		if nil != err {
			// Report failure for metrics
			// obs.droppedNetworkErrCounter.Add(1.0)
		} else {
			// Report Result for metrics
			// code = strconv.Itoa(resp.StatusCode)

			// read until the response is complete before closing to allow
			// connection reuse
			if nil != resp.Body {
				io.Copy(ioutil.Discard, resp.Body)
				resp.Body.Close()
			}
		}
		// obs.deliveryCounter.With("url", obs.id, "code", code, "event", event).Add(1.0)
		// add delivery metric
	}

}

// called if queue is filled
func (d Dispatcher) queueOverflow() {
	d.Empty()
	// add metrics

	// add cutoffs
}

// update TTL for norn
func (d Dispatcher) Update(norn model.Norn) {

}

func (d Dispatcher) Stop() {
	close(d.DispatchQueue.Load().(chan *EventNorn))
}
