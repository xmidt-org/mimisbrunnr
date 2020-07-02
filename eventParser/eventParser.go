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
	"time"

	"emperror.dev/emperror"
	"github.com/go-kit/kit/log"
	db "github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/mimisbrunnr/norn"
	"github.com/xmidt-org/svalinn/rules"
	"github.com/xmidt-org/webpa-common/logging"
	semaphore "github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/v2"
)

const (
	defaultTTL          = time.Duration(5) * time.Minute
	minMaxWorkers       = 5
	defaultMinQueueSize = 5
)

type Options struct {
	QueueSize  int
	MaxWorkers int
	RegexRules []rules.RuleConfig
}

type eventParser struct {
	parserRules  rules.Rules
	requestQueue chan *wrp.Message
	parseWorkers semaphore.Interface
	logger       log.Logger
	measures     *Measures
	wg           sync.WaitGroup
	opt          Options
	sender       norn.EventSender
}

func NewEventParser(sender norn.EventSender, logger log.Logger, o Options) (*eventParser, error) { //{ config EventParserConfig)
	if o.MaxWorkers < minMaxWorkers {
		o.MaxWorkers = minMaxWorkers
	}
	if o.QueueSize < defaultMinQueueSize {
		o.QueueSize = defaultMinQueueSize
	}
	queue := make(chan *wrp.Message, o.QueueSize)
	workers := semaphore.New(o.MaxWorkers)

	parsedRules, err := rules.NewRules(o.RegexRules)
	if err != nil {
		return nil, emperror.Wrap(err, "failed to create rules from config")
	}

	eventParser := eventParser{
		parserRules:  parsedRules,
		requestQueue: queue,
		parseWorkers: workers,
		opt:          o,
		sender:       sender,
	}
	return &eventParser, nil
}

func (p *eventParser) eventHandler(writer http.ResponseWriter, req *http.Request) {
	var message *wrp.Message
	msgBytes, err := ioutil.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		logging.Error(p.logger).Log(logging.MessageKey(), "Could not read request body", logging.ErrorKey(), err.Error())
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	err = wrp.NewDecoderBytes(msgBytes, wrp.Msgpack).Decode(&message)
	if err != nil {
		logging.Error(p.logger).Log(logging.MessageKey(), "Could not decode request body", logging.ErrorKey(), err.Error())
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// add message to queue
	select {
	case p.requestQueue <- message:
		if p.measures != nil {
			p.measures.EventParsingQueue.Add(1.0)
		}
	default:
		if p.measures != nil {
			p.measures.DroppedEventsParsingCount.With(reasonLabel, queueFullReason).Add(1.0)
		}
		errors.New("Event Parser Queue Full")
	}

	writer.WriteHeader(http.StatusAccepted)
}

func (p *eventParser) Start() func(context.Context) error {

	return func(ctx context.Context) error {
		p.wg.Add(1)
		go p.parseEvents()
		return nil
	}
}

func (p *eventParser) parseEvents() {
	var (
		message *wrp.Message
	)
	select {
	case message = <-p.requestQueue:
		if p.measures != nil {
			p.measures.EventParsingQueue.Add(-1.0)
		}
		p.parseWorkers.Acquire()
		go p.parseDeviceID(message)
	}

	for i := 0; i < p.opt.MaxWorkers; i++ {
		p.parseWorkers.Acquire()
	}
}

func (p *eventParser) parseDeviceID(message *wrp.Message) error {
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
	p.sender.Send(message, deviceID)

	return err

}

func (p *eventParser) Stop() func(context.Context) error {
	return func(ctx context.Context) error {
		close(p.requestQueue)
		p.wg.Wait()
		return nil
	}

}
