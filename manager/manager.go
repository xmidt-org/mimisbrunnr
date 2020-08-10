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
	nornsDispatch    map[string]nornDispatcher
	dispatcherConfig dispatch.DispatcherConfig
	logger           log.Logger
	mutex            sync.RWMutex
}

type nornDispatcher struct {
	norn       model.Norn
	dispatcher dispatch.D
}

func NewManager(dc dispatch.DispatcherConfig, logger log.Logger) (*Manager, error) {
	return &Manager{
		nornsDispatch:    map[string]nornDispatcher{},
		dispatcherConfig: dc,
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
	newNorns := []nornDispatcher{}
	oldNorns := []dispatch.D{}

	transport := NewTransport(m.dispatcherConfig)

	for _, item := range items {
		id := item.Identifier
		norn, err := model.ConvertItemToNorn(item)
		if err != nil {
			log.WithPrefix(m.logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Norn", "item", item)
		}
		if _, ok := m.nornsDispatch[id]; ok {
			recentMap[id] = norn
		} else {
			dispatcher, err := dispatch.NewDispatcher(m.dispatcherConfig, norn, transport)
			if err == nil {
				newNorns = append(newNorns, nornDispatcher{norn: norn, dispatcher: dispatcher})
				recentMap[id] = norn
			} else {
				m.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), err.Error)
			}
		}
	}

	for i, norndis := range m.nornsDispatch {
		if val, ok := recentMap[i]; !ok {
			oldNorns = append(oldNorns, norndis.dispatcher)
			delete(m.nornsDispatch, i)
		} else {
			norndis.dispatcher.Update(val)
		}
	}

	for _, norndis := range newNorns {
		m.mutex.Lock()
		m.nornsDispatch[norndis.norn.DeviceID] = norndis
		m.mutex.Unlock()
	}

	for _, dispatcher := range oldNorns {
		dispatcher.Stop(nil)
	}

}

func (m *Manager) Send(deviceID string, event *wrp.Message) {
	m.mutex.RLock()
	for _, nd := range m.nornsDispatch {
		nd.dispatcher.Dispatch(deviceID, event)
	}
	m.mutex.Unlock()
}

func NewGetEndpoint(m *Manager) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			idOwner norn.IdOwnerItem
			ok      bool
			norndis nornDispatcher
		)
		if idOwner, ok = request.(norn.IdOwnerItem); !ok {
			return nil, norn.BadRequestError{Request: request}
		}
		if norndis, ok = m.nornsDispatch[idOwner.ID]; ok {
			return norndis.norn, nil
		}
		return norndis.norn, nil
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
