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
	"crypto/tls"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	aws "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/semaphore"
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
	DeliverUntil          time.Time
}

type Dispatcher struct {
	DispatchQueue    atomic.Value
	Measures         *Measures
	Workers          semaphore.Interface
	MaxWorkers       int
	Logger           log.Logger
	DeliveryRetries  int
	DeliveryInterval time.Duration
	QueueSize        int
	Sender           func(*http.Request) (*http.Response, error)
	Mutex            sync.RWMutex
	DropUntil        time.Time
	CutOffPeriod     time.Duration
	FailureMsg       FailureMessage
	Wg               sync.WaitGroup
	DeliverUntil     time.Time
	Norn             model.Norn
	DestinationType  string
	SqsClient        *sqs.SQS
}

type eventWithID struct {
	Event    *wrp.Message
	DeviceID string
}

type FailureMessage struct {
	Text         string `json:"text"`
	CutOffPeriod string `json:"cut_off_period"`
	QueueSize    int    `json:"queue_size"`
	Workers      int    `json:"worker_count"`
}

const (
	HttpType = "http"
	SqsType  = "sqs"
)

func NewTransport(dc DispatcherConfig) *http.Transport {
	return &http.Transport{
		TLSClientConfig:       &tls.Config{},
		MaxIdleConnsPerHost:   dc.SenderConfig.NumWorkersPerSender,
		ResponseHeaderTimeout: dc.SenderConfig.ResponseHeaderTimeout,
		IdleConnTimeout:       dc.SenderConfig.IdleConnTimeout,
	}
}

func NewDispatcher(dc DispatcherConfig, norn model.Norn, transport *http.Transport) (D, error) {
	if dc.QueueSize < defaultMinQueueSize {
		dc.QueueSize = defaultMinQueueSize
	}

	if dc.NumWorkers < minMaxWorkers {
		dc.NumWorkers = minMaxWorkers
	}

	dispatcher := Dispatcher{
		MaxWorkers: dc.NumWorkers,
		QueueSize:  dc.QueueSize,
		Sender: (&http.Client{
			Transport: transport,
		}).Do,
		FailureMsg: FailureMessage{
			Text:         FailureText,
			CutOffPeriod: dc.SenderConfig.CutOffPeriod.String(),
			QueueSize:    dc.QueueSize,
			Workers:      dc.NumWorkers,
		},
		DeliverUntil: dc.SenderConfig.DeliverUntil,
		Norn:         norn,
	}

	dispatcher.DispatchQueue.Store(make(chan *eventWithID, dc.QueueSize))

	if (norn.Destination.AWSConfig) == (model.AWSConfig{}) {
		dispatcher.DestinationType = HttpType
	} else {
		dispatcher.DestinationType = SqsType

		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(norn.Destination.AWSConfig.Sqs.Region),
			Credentials: credentials.NewStaticCredentials(norn.Destination.AWSConfig.ID, norn.Destination.AWSConfig.AccessKey, norn.Destination.AWSConfig.SecretKey),
		})
		if err != nil {
			return dispatcher, err
		}
		dispatcher.SqsClient = sqs.New(sess)
	}

	return dispatcher, nil
}

func NewEventWithID(event *wrp.Message, deviceID string) *eventWithID {
	return &eventWithID{
		Event:    event,
		DeviceID: deviceID,
	}
}
