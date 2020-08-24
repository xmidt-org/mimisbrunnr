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
	"crypto/tls"
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

type Manager struct {
	idFilter         map[string]filterDispatcher
	urlDispatcher    map[string]filterDispatcher
	dispatcherConfig dispatch.SenderConfig
	filterConfig     dispatch.FilterConfig
	logger           log.Logger
	mutex            sync.RWMutex
	measures         dispatch.Measures
	sender           *http.Client
}

type filterDispatcher struct {
	norn       model.Norn
	dispatcher dispatch.D
	filter     Filterer
}

type Filterer interface {
	Start(context.Context) error
	Filter(deviceID string, event *wrp.Message)
	Update(norn model.Norn)
	Stop(context.Context) error
}

func NewManager(dc dispatch.SenderConfig, fc dispatch.FilterConfig, logger log.Logger, measures dispatch.Measures) (*Manager, error) {
	transport := NewTransport(dc)
	sender := &http.Client{
		Transport: transport,
	}

	return &Manager{
		idFilter:         map[string]filterDispatcher{},
		urlDispatcher:    map[string]filterDispatcher{},
		dispatcherConfig: dc,
		filterConfig:     fc,
		logger:           logger,
		measures:         measures,
		sender:           sender,
	}, nil
}

func NewTransport(dc dispatch.SenderConfig) http.RoundTripper {
	var transport http.RoundTripper = &http.Transport{
		TLSClientConfig:       &tls.Config{},
		MaxIdleConnsPerHost:   dc.NumWorkersPerSender,
		ResponseHeaderTimeout: dc.ResponseHeaderTimeout,
		IdleConnTimeout:       dc.IdleConnTimeout,
	}
	return transport
}

// chrysom client listener
func (m *Manager) Update(items []argus.Item) {
	recentIDMap := make(map[string]model.Norn)
	recentURLMap := make(map[string]model.Norn)

	newNorns := []filterDispatcher{}
	oldNorns := []Filterer{}

	var dispatcher dispatch.D
	var filter *dispatch.Filter
	var url string

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

		if _, ok := m.idFilter[id]; ok {
			recentIDMap[id] = norn
		} else {
			filter, err = dispatch.NewFilter(m.filterConfig, dispatcher, norn, m.sender)
			if err != nil {
				m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
			} else {
				recentIDMap[id] = norn
			}

		}

		if _, ok := m.urlDispatcher[url]; ok {
			recentURLMap[url] = norn
		} else {
			if (norn.Destination.AWSConfig) == (model.AWSConfig{}) {
				dispatcher, err = dispatch.NewHttpDispatcher(m.dispatcherConfig, norn.Destination.HttpConfig, m.sender, m.logger, m.measures)
				if err != nil {
					m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
				} else {
					newNorns = append(newNorns, filterDispatcher{norn: norn, dispatcher: dispatcher, filter: filter})
					recentURLMap[url] = norn
				}
			} else {
				dispatcher, err = dispatch.NewSqsDispatcher(m.dispatcherConfig, norn.Destination.AWSConfig, m.logger, m.measures)
				if err != nil {
					m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
				} else {
					newNorns = append(newNorns, filterDispatcher{norn: norn, dispatcher: dispatcher, filter: filter})
					recentURLMap[url] = norn
				}
			}
		}
	}

	for i, filterdis := range m.idFilter {
		if norn, ok := recentIDMap[i]; !ok {
			oldNorns = append(oldNorns, filterdis.filter)
			delete(m.idFilter, i)
		} else {
			filterdis.filter.Update(norn)
		}
	}

	for i, filterdis := range m.urlDispatcher {
		if norn, ok := recentURLMap[i]; !ok {
			oldNorns = append(oldNorns, filterdis.filter)
			delete(m.urlDispatcher, i)
		} else {
			filterdis.dispatcher.Update(norn)
		}
	}

	for _, filterdis := range newNorns {
		m.mutex.Lock()
		m.idFilter[filterdis.norn.DeviceID] = filterdis
		m.mutex.Unlock()
	}

	for _, filter := range oldNorns {
		filter.Stop(nil)
	}

}

func (m *Manager) Send(deviceID string, event *wrp.Message) {
	m.mutex.RLock()
	for _, fd := range m.idFilter {
		fd.filter.Filter(deviceID, event)
	}
	m.mutex.Unlock()
}

func NewGetEndpoint(m *Manager) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			idOwner    norn.IdOwnerItem
			ok         bool
			nornFilter filterDispatcher
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

func NewGetEndpointDecode() kithttp.DecodeRequestFunc {
	return func(ctx context.Context, req *http.Request) (interface{}, error) {
		nornID := mux.Vars(req)
		id := nornID["id"]
		return &norn.IdOwnerItem{
			ID: id,
		}, nil
	}
}
