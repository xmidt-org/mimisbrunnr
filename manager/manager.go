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

package manager

import (
	"context"
	"net/http"
	"sync"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	argus "github.com/xmidt-org/argus/model"
	"github.com/xmidt-org/mimisbrunnr/dispatch"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/mimisbrunnr/norn"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/v2"
)

// Manager is in charge of fanning out events to its respective dispatcher (HTTP or Sqs).
// It keeps track of all recent Filterers and Dispatchers part of its map and is
// responsible for updating or removing them.
type Manager struct {
	idFilter         map[string]nornFilter
	urlDispatcher    map[string]dispatch.D
	dispatcherConfig *dispatch.DispatcherSender
	filterConfig     *dispatch.FilterSender
	logger           log.Logger
	mutex            sync.RWMutex
	measures         dispatch.Measures
	sender           *http.Client
}

type nornFilter struct {
	norn   model.Norn
	filter Filterer
}

type endpointDispatcher struct {
	endpoint   string
	dispatcher dispatch.D
	filter     Filterer
}

// Filterer is used to filter events by deviceID.
type Filterer interface {
	// Start begins pulling from the filter queue to deliver events.
	Start(context.Context) error

	// Filter checks if the event's deviceID matches deviceID of norn
	// and queue it accordingly.
	Filter(deviceID string, event *wrp.Message)

	// Update will update the time a norn expires.
	Update(norn model.Norn)

	// Stop closes the filter queue and resets its metric.
	Stop(context.Context) error
}

// NewManager constructs a Manager from the provided configs.
func NewManager(sc dispatch.SenderConfig, transport http.RoundTripper, logger log.Logger, measures dispatch.Measures) (*Manager, error) {
	sender := &http.Client{
		Transport: transport,
	}

	return &Manager{
		idFilter:      map[string]nornFilter{},
		urlDispatcher: map[string]dispatch.D{},
		dispatcherConfig: &dispatch.DispatcherSender{
			DeliveryInterval: sc.DeliveryInterval,
			DeliveryRetries:  sc.DeliveryRetries,
		},
		filterConfig: &dispatch.FilterSender{
			QueueSize:  sc.FilterQueueSize,
			NumWorkers: sc.MaxWorkers,
		},
		logger:   logger,
		measures: measures,
		sender:   sender,
	}, nil
}

// Update is the argus chrysom client listener. For every recent item that is provided
// by argus, Update will check if the dispatcher and filter for an item already exists
// as part of the manager's local cache (idFilter and urlDispatcher maps) and update cache
// with the recent norn retrieved from item. If the norn is brand new, a new dispatcher and
// filter is created for it and added to local cache. Any old dispatchers and filters is also
// removed part of Update.
func (m *Manager) Update(items []argus.Item) {
	recentIDMap := make(map[string]model.Norn)
	recentURLMap := make(map[string]model.Norn)

	newNorns := []nornFilter{}
	newDispatchers := []endpointDispatcher{}
	oldFilterNorns := []Filterer{}
	oldDispatcherUrls := []dispatch.D{}

	var (
		dispatcher dispatch.D
		filter     *dispatch.Filter
		url        string
	)

	for _, item := range items {
		id := item.Identifier
		norn, err := model.ConvertItemToNorn(item)
		if err != nil {
			log.WithPrefix(m.logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Norn", "item", item)
		}
		if (norn.Destination.AWSConfig) == (model.AWSConfig{}) {
			url = norn.Destination.HttpConfig.URL
		} else {
			url = norn.Destination.AWSConfig.Sqs.QueueURL
		}

		if _, ok := m.urlDispatcher[url]; ok {
			recentURLMap[url] = norn
		} else {
			if (norn.Destination.AWSConfig) == (model.AWSConfig{}) {
				dispatcher, err = dispatch.NewHTTPDispatcher(m.dispatcherConfig, norn.Destination.HttpConfig, m.sender, m.logger, m.measures)
				if err != nil {
					m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
				} else {
					dispatcher.Start(nil)
					newDispatchers = append(newDispatchers, endpointDispatcher{endpoint: url, dispatcher: dispatcher})
					recentURLMap[url] = norn
				}
			} else {
				dispatcher, err = dispatch.NewSqsDispatcher(m.dispatcherConfig, norn.Destination.AWSConfig, m.logger, m.measures)
				if err != nil {
					m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
				} else {
					dispatcher.Start(nil)
					newDispatchers = append(newDispatchers, endpointDispatcher{endpoint: url, dispatcher: dispatcher})
					recentURLMap[url] = norn
				}
			}
		}

		if _, ok := m.idFilter[id]; ok {
			recentIDMap[id] = norn
		} else {
			filter, err = dispatch.NewFilter(m.filterConfig, dispatcher, norn, m.sender, m.measures)
			if err != nil {
				m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
			} else {
				filter.Start(nil)
				newNorns = append(newNorns, nornFilter{norn: norn, filter: filter})
				recentIDMap[id] = norn
			}
		}
	}

	for i, nornFilter := range m.idFilter {
		if norn, ok := recentIDMap[i]; !ok {
			oldFilterNorns = append(oldFilterNorns, nornFilter.filter)
			m.mutex.Lock()
			delete(m.idFilter, i)
			m.mutex.Unlock()
		} else {
			nornFilter.filter.Update(norn)
		}
	}

	for i, dispatcher := range m.urlDispatcher {
		if norn, ok := recentURLMap[i]; !ok {
			oldDispatcherUrls = append(oldDispatcherUrls, dispatcher)
			m.mutex.Lock()
			delete(m.urlDispatcher, i)
			m.mutex.Unlock()
		} else {
			dispatcher.Update(norn)
		}
	}

	for _, nornFilter := range newNorns {
		m.mutex.Lock()
		m.idFilter[nornFilter.norn.DeviceID] = nornFilter
		m.mutex.Unlock()
	}

	for _, endpointDispatcher := range newDispatchers {
		m.mutex.Lock()
		m.urlDispatcher[endpointDispatcher.endpoint] = endpointDispatcher.dispatcher
		m.mutex.Unlock()
	}

	for _, filter := range oldFilterNorns {
		filter.Stop(nil)
	}

}

// Send will send the message to Filter for it to be checked if norn's deviceID
// matches the event's.
func (m *Manager) Send(deviceID string, event *wrp.Message) {
	m.mutex.RLock()
	for _, fd := range m.idFilter {
		fd.filter.Filter(deviceID, event)
	}
	m.mutex.Unlock()
}

// NewGetEndpoint returns the endpoint for /norns/{id} handler.
func NewGetEndpoint(m *Manager) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			idOwner    norn.IdOwnerItem
			ok         bool
			nornFilter nornFilter
		)
		if idOwner, ok = request.(norn.IdOwnerItem); !ok {
			return nil, norn.BadRequestError{Request: request}
		}
		if nornFilter, ok = m.idFilter[idOwner.ID]; ok {
			return nornFilter.norn, nil
		}
		return nornFilter.norn, nil
	}

}

// NewGetEndpointDecode returns DecodeRequestFunc wrapper from the /norns/{id} endpoint.
func NewGetEndpointDecode() kithttp.DecodeRequestFunc {
	return func(ctx context.Context, req *http.Request) (interface{}, error) {
		nornID := mux.Vars(req)
		id := nornID["id"]
		return &norn.IdOwnerItem{
			ID: id,
		}, nil
	}
}
