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
	nornFilter       map[string]filterDispatcher
	dispatcherConfig dispatch.DispatcherConfig
	filterConfig     dispatch.FilterConfig
	logger           log.Logger
	mutex            sync.RWMutex
}

type filterDispatcher struct {
	norn       model.Norn
	dispatcher dispatch.D
	filter     dispatch.F
}

func NewManager(dc dispatch.DispatcherConfig, fc dispatch.FilterConfig, logger log.Logger) (*Manager, error) {
	return &Manager{
		nornFilter:       map[string]filterDispatcher{},
		dispatcherConfig: dc,
		filterConfig:     fc,
		logger:           logger,
	}, nil
}

func NewTransport(dc dispatch.DispatcherConfig) http.RoundTripper {
	var transport http.RoundTripper = &http.Transport{
		TLSClientConfig:       &tls.Config{},
		MaxIdleConnsPerHost:   dc.SenderConfig.NumWorkersPerSender,
		ResponseHeaderTimeout: dc.SenderConfig.ResponseHeaderTimeout,
		IdleConnTimeout:       dc.SenderConfig.IdleConnTimeout,
	}
	return transport
}

// chrysom client listener
func (m *Manager) Update(items []argus.Item) {
	recentMap := make(map[string]model.Norn)
	newNorns := []filterDispatcher{}
	oldNorns := []dispatch.F{}

	transport := NewTransport(m.dispatcherConfig)
	sender := &http.Client{
		Transport: transport,
	}
	var dispatcher dispatch.D

	for _, item := range items {
		id := item.Identifier
		norn, err := model.ConvertItemToNorn(item)
		if err != nil {
			log.WithPrefix(m.logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Norn", "item", item)
		}
		if _, ok := m.nornFilter[id]; ok {
			recentMap[id] = norn
		} else {

			if (norn.Destination.AWSConfig) == (model.AWSConfig{}) {
				dispatcher, err = dispatch.NewHttpDispatcher(m.dispatcherConfig, norn, sender)
				if err != nil {
					m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
				}
			} else {
				dispatcher, err = dispatch.NewSqsDispatcher(m.dispatcherConfig, norn, transport)
				if err != nil {
					m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
				}
			}

			filter, err := dispatch.NewFilter(m.filterConfig, dispatcher, norn, sender)
			if err != nil {
				m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
			}
			if err == nil {
				newNorns = append(newNorns, filterDispatcher{norn: norn, filter: filter})
				recentMap[id] = norn
			} else {
				m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
			}

		}
	}

	for i, filterdis := range m.nornFilter {
		if norn, ok := recentMap[i]; !ok {
			oldNorns = append(oldNorns, filterdis.filter)
			delete(m.nornFilter, i)
		} else {
			filterdis.dispatcher.Update(norn)
			filterdis.filter.Update(norn)
		}
	}

	for _, filterdis := range newNorns {
		m.mutex.Lock()
		m.nornFilter[filterdis.norn.DeviceID] = filterdis
		m.mutex.Unlock()
	}

	for _, dispatcher := range oldNorns {
		dispatcher.Stop(nil)
	}
	for _, filter := range oldNorns {
		filter.Stop(nil)
	}

}

func (m *Manager) Send(deviceID string, event *wrp.Message) {
	m.mutex.RLock()
	for _, fd := range m.nornFilter {
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
		if nornFilter, ok = m.nornFilter[idOwner.ID]; ok {
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
