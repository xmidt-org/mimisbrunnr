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
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/wrp-go/v2"
)

const (
	defaultMinQueueSize = 5
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
	queueSize int
}

type Dispatcher struct {
	DispatchQueue chan *EventNorn
}

type EventNorn struct {
	Event    *wrp.Message
	Norn     model.Norn
	DeviceID string
}

func NewDispatcher(dc DispatcherConfig) (D, error) {
	var size int
	if dc.queueSize < defaultMinQueueSize {
		size = defaultMinQueueSize
	}
	queue := make(chan *EventNorn, size)

	return Dispatcher{
		DispatchQueue: queue,
	}, nil
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
	case d.DispatchQueue <- en:
		// add metric
	default:
		overflow()
		// add metric
	}
}

func (d Dispatcher) Dispatch() error {
	for {
		select {
		case en, ok := <-d.DispatchQueue:
			if !ok {
				break
			}
			if en.DeviceID == en.Norn.DeviceID {
				go send(en.Event)
			}
		}

	}
}

// update TTL for norn
func (d Dispatcher) Update(norn model.Norn) {

}

func (d Dispatcher) Stop() {
	close(d.DispatchQueue)
}

// called if queue is filled
func overflow() {

}

//called to deliver event
func send(msg *wrp.Message) {

}
