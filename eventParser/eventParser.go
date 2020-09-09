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
	"errors"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"

	"emperror.dev/emperror"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	kithttp "github.com/go-kit/kit/transport/http"
	db "github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/mimisbrunnr/norn"
	"github.com/xmidt-org/svalinn/rules"
	"github.com/xmidt-org/webpa-common/logging"
	semaphore "github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/v2"
)

const (
	MinMaxWorkers       = 5
	DefaultMinQueueSize = 5
	reasonLabel         = "reason"
	queueFullReason     = "queue_full"
)

// EventSenderFunc is the function type used pass events to manager.
type EventSenderFunc func(deviceID string, event *wrp.Message)

// ParserConfig is the config provided to create a new EventParser
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
	measures     Measures
	wg           sync.WaitGroup
	opt          ParserConfig
	sender       EventSenderFunc
}

type Response struct {
	response int
}

// NewEventParser validates and constructs an EventParser from provided configs
func NewEventParser(sender EventSenderFunc, logger *log.Logger, o ParserConfig, measures Measures) (*EventParser, error) {
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

	if (measures == Measures{}) {
		return nil, errors.New("measures not set for parser")
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

// NewEventsEndpoint returns the endpoint for /events handler
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
			p.measures.EventParsingQueue.Add(1.0)
		default:
			p.measures.DroppedEventsParsingCount.With(reasonLabel, queueFullReason).Add(1.0)
			return &Response{
				response: http.StatusTooManyRequests,
			}, nil
		}
		return &Response{
			response: http.StatusOK,
		}, nil
	}
}

// NewEventsEndpointDecode returns DecodeRequestFunc wrapper from the /events endpoint
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

// NewEventsEndpointEncode returns EncodeResponseFunc wrapper from the /events endpoint
func NewEventsEndpointEncode() kithttp.EncodeResponseFunc {
	return func(ctx context.Context, resp http.ResponseWriter, value interface{}) error {
		if value != nil {
			resp.WriteHeader(value.(Response).response)
		}
		return nil
	}
}

// Start creates the queue and begins to pull messages from it
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
		p.parseWorkers.Acquire()
		err := p.parseDeviceID(message)
		if err != nil {
			p.logger.Log(level.Key(), level.InfoValue, logging.MessageKey(), "Failed to parse ID")
		}
	}

	for i := 0; i < p.opt.MaxWorkers; i++ {
		p.parseWorkers.Acquire()
	}
}

// parseDeviceID uses the configured regex rules to parse the deviceID based on the event type.
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
		base, id := path.Split(path.Base(base))
		if id == "" {
			return err
		}
		deviceID = strings.ToLower(id)
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

// Stop closes the parser queue
func (p *EventParser) Stop() func(context.Context) error {
	return func(ctx context.Context) error {
		close(p.requestQueue.Load().(chan *wrp.Message))
		p.wg.Wait()
		return nil
	}

}
