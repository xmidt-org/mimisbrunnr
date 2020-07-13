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

	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/v2"
)

type DispatcherConfig struct {
	QueueSize    int
	NumWorkers   int
	SenderConfig SenderConfig
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
}

type EventNorn struct {
	Event    *wrp.Message
	Norn     model.Norn
	DeviceID string
}

type FailureMessage struct {
	Text         string `json:"text"`
	CutOffPeriod string `json:"cut_off_period"`
	QueueSize    int    `json:"queue_size"`
	Workers      int    `json:"worker_count"`
}

func NewDispatcher(dc DispatcherConfig) (D, error) {
	if dc.QueueSize < defaultMinQueueSize {
		dc.QueueSize = defaultMinQueueSize
	}

	if dc.NumWorkers < minMaxWorkers {
		dc.NumWorkers = minMaxWorkers
	}

	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		MaxIdleConnsPerHost:   dc.SenderConfig.NumWorkersPerSender,
		ResponseHeaderTimeout: dc.SenderConfig.ResponseHeaderTimeout,
		IdleConnTimeout:       dc.SenderConfig.IdleConnTimeout,
	}

	dispatcher := Dispatcher{
		MaxWorkers: dc.NumWorkers,
		QueueSize:  dc.QueueSize,
		Sender: (&http.Client{
			Transport: tr,
		}).Do,
		FailureMsg: FailureMessage{
			Text:         FailureText,
			CutOffPeriod: dc.SenderConfig.CutOffPeriod.String(),
			QueueSize:    dc.QueueSize,
			Workers:      dc.NumWorkers,
		},
		DeliverUntil: dc.SenderConfig.DeliverUntil,
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
