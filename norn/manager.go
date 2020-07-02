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

package norn

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	argus "github.com/xmidt-org/argus/model"
	"github.com/xmidt-org/mimisbrunnr/dispatch"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/v2"
)

type Manager struct {
	nornsDispatch    map[string]nornDispatcher
	dispatcherConfig dispatch.DispatcherConfig
	logger           log.Logger
}

type nornDispatcher struct {
	norn       model.Norn
	dispatcher dispatch.D
}

type Listener interface {
	Update(items []argus.Item)
}

type EventSender interface {
	Send(event *wrp.Message, deviceID string) //send event to all dispatchers in map
}

// func newDispatcher(dc dispatch.DispatcherConfig) (dispatch.Dispatcher, error) {
// 	// create new dispatcher for
// 	// var df dispatch.DispatchFunc

// 	return df
// }

func (m *Manager) Update(items []argus.Item) {
	recentMap := make(map[string]model.Norn)
	newNorns := []nornDispatcher{}
	oldNorns := []dispatch.D{}

	for _, item := range items {
		id := item.Identifier
		norn, err := ConvertItemToNorn(item)
		if err != nil {
			log.WithPrefix(m.logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Norn", "item", item)
		}
		if _, ok := m.nornsDispatch[id]; ok {
			recentMap[id] = norn
		} else {
			dispatcher, err := dispatch.NewDispatcher(m.dispatcherConfig)
			if err == nil {
				newNorns = append(newNorns, nornDispatcher{norn: norn, dispatcher: dispatcher})
				recentMap[id] = norn
			}
		}
	}

	for i, norndis := range m.nornsDispatch {
		if val, ok := recentMap[i]; !ok {
			oldNorns = append(oldNorns, norndis.dispatcher)
			delete(m.nornsDispatch, i)
		} else {
			// update TTL for norn
			norndis.dispatcher.Update(val)
		}
	}

	for _, norndis := range newNorns {
		m.nornsDispatch[norndis.norn.DeviceID] = norndis
	}

	for _, dispatcher := range oldNorns {
		dispatcher.Stop()
	}

}

func (m *Manager) Send(event *wrp.Message, deviceID string) {
	for _, nd := range m.nornsDispatch {
		// call Queue()
		en := dispatch.NewEventNorn(event, nd.norn, deviceID)
		nd.dispatcher.Queue(*en)
		nd.dispatcher.Dispatch(event, nd.norn, deviceID)
	}
}

// GET '/norns'
func (r *Registry) GetAllNorns(rw http.ResponseWriter, req *http.Request) (norns []model.Norn, err error) {
	items, err := r.hookStore.GetItems("")
	if err != nil {
		return
	}
	norns = []model.Norn{}
	for _, item := range items {
		norn, err := ConvertItemToNorn(item)
		if err != nil {
			log.WithPrefix(r.config.Logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Norn", "item", item)
			continue
		}
		norns = append(norns, norn)
	}
	return norns, nil
}

// GET '/norns/id'
func (m *Manager) GetNorn(rw http.ResponseWriter, req *http.Request) (norn model.Norn, err error) {
	nornID := mux.Vars(req)
	id := nornID["id"]

	if norndis, ok := m.nornsDispatch[id]; ok {
		return norndis.norn, nil
	} else {
		logging.Info(m.logger).Log(logging.MessageKey(), "Could not get norn", logging.ErrorKey(), err)
	}
	return
}
