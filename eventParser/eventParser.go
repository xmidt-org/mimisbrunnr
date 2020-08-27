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

package eventParser

import (
	"context"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"emperror.dev/emperror"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
	db "github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/mimisbrunnr/norn"
	"github.com/xmidt-org/svalinn/rules"
	"github.com/xmidt-org/webpa-common/logging"
	semaphore "github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/v2"
)

const (
	DefaultTTL          = time.Duration(5) * time.Minute
	MinMaxWorkers       = 5
	DefaultMinQueueSize = 5
	reasonLabel         = "reason"
	queueFullReason     = "queue_full"
)

type EventSenderFunc func(deviceID string, event *wrp.Message)

type ParserConfig struct {
	QueueSize  int
	MaxWorkers int
	RegexRules []rules.RuleConfig
}

type EventParser struct {
	parserRules  rules.Rules
	requestQueue atomic.Value
	parseWorkers semaphore.Interface
	logger       log.Logger
	measures     *Measures
	wg           sync.WaitGroup
	opt          ParserConfig
	sender       EventSenderFunc
}

type Response struct {
	response int
}

func NewEventParser(sender EventSenderFunc, logger *log.Logger, o ParserConfig, measures *Measures) (*EventParser, error) {
	if o.MaxWorkers < MinMaxWorkers {
		o.MaxWorkers = MinMaxWorkers
	}
	if o.QueueSize < DefaultMinQueueSize {
		o.QueueSize = DefaultMinQueueSize
	}
	workers := semaphore.New(o.MaxWorkers)

	parsedRules, err := rules.NewRules(o.RegexRules)
	if err != nil {
		return nil, emperror.Wrap(err, "failed to create rules from config")
	}

	eventParser := EventParser{
		parserRules:  parsedRules,
		parseWorkers: workers,
		opt:          o,
		sender:       sender,
		measures:     measures,
	}
	return &eventParser, nil
}

func NewEventsEndpoint(p *EventParser) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			message *wrp.Message
			ok      bool
		)
		if message, ok = request.(*wrp.Message); !ok {
			return nil, norn.BadRequestError{Request: request}
		}

		select {
		case p.requestQueue.Load().(chan *wrp.Message) <- message:
			if p.measures != nil {
				p.measures.EventParsingQueue.Add(1.0)
			}
		default:
			if p.measures != nil {
				p.measures.DroppedEventsParsingCount.With(reasonLabel, queueFullReason).Add(1.0)
			}
			return &Response{
				response: http.StatusTooManyRequests,
			}, nil
		}
		return &Response{
			response: http.StatusOK,
		}, nil
	}
}

func NewEventsEndpointDecode() kithttp.DecodeRequestFunc {
	return func(ctx context.Context, req *http.Request) (interface{}, error) {
		var message *wrp.Message
		msgBytes, err := ioutil.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}

		err = wrp.NewDecoderBytes(msgBytes, wrp.Msgpack).Decode(&message)
		if err != nil {
			return nil, err
		}
		return message, nil
	}
}

func NewEventsEndpointEncode() kithttp.EncodeResponseFunc {
	return func(ctx context.Context, resp http.ResponseWriter, value interface{}) error {
		if value != nil {
			resp.WriteHeader(value.(Response).response)
		}
		return nil
	}
}

func (p *EventParser) Start() func(context.Context) error {

	return func(ctx context.Context) error {

		p.requestQueue.Store(make(chan *wrp.Message, p.opt.QueueSize))
		p.wg.Add(1)
		go p.parseEvents()
		return nil
	}
}

func (p *EventParser) parseEvents() {
	defer p.wg.Done()
	var (
		message *wrp.Message
	)
	queue := p.requestQueue.Load().(chan *wrp.Message)
	select {
	case message = <-queue:
		p.measures.EventParsingQueue.Add(-1.0)
		// if p.measures != nil {
		// 	p.measures.EventParsingQueue.Add(-1.0)
		// }
		p.parseWorkers.Acquire()
		go p.parseDeviceID(message)
	}

	for i := 0; i < p.opt.MaxWorkers; i++ {
		p.parseWorkers.Acquire()
	}
}

func (p *EventParser) parseDeviceID(message *wrp.Message) error {
	var (
		err      error
		deviceID string
	)

	rules, err := rules.NewRules(p.opt.RegexRules)
	if err != nil {
		return err
	}

	rule, err := rules.FindRule(message.Destination)
	if err != nil {
		logging.Info(p.logger).Log(logging.MessageKey(), "Could not get rule", logging.ErrorKey(), err, "destination", message.Destination)
	}

	eventType := db.Default
	if rule != nil {
		eventType = db.ParseEventType(rule.EventType())
	}

	if eventType == db.State {
		// get state and id from dest if this is a state event
		base, _ := path.Split(message.Destination)
		base, deviceId := path.Split(path.Base(base))
		if deviceId == "" {
			return err
		}
		deviceID = strings.ToLower(deviceId)
	} else {
		if message.Source == "" {
			return err
		}
		deviceID = strings.ToLower(message.Source)
	}

	// call manager
	p.sender(deviceID, message)

	return err

}

func (p *EventParser) Stop() func(context.Context) error {
	return func(ctx context.Context) error {
		close(p.requestQueue.Load().(chan *wrp.Message))
		p.wg.Wait()
		return nil
	}

}
