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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/xmidt-org/argus/chrysom"
	argus "github.com/xmidt-org/argus/model"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/logging"
)

type Registry struct {
	hookStore *chrysom.Client
	config    RegistryConfig
}

type RegistryConfig struct {
	Logger      log.Logger
	Listener    chrysom.ListenerFunc
	ArgusConfig chrysom.ClientConfig
}

func jsonResponse(rw http.ResponseWriter, code int, msg string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)
	rw.Write([]byte(fmt.Sprintf(`{"message":"%s"}`, msg)))
}

func NewRegistry(config RegistryConfig, listener chrysom.Listener) (*Registry, error) {
	argus, err := chrysom.CreateClient(config.ArgusConfig, chrysom.WithLogger(config.Logger))
	if err != nil {
		return nil, err
	}
	if listener != nil {
		argus.SetListener(listener)
	}
	return &Registry{
		config:    config,
		hookStore: argus,
	}, nil
}

func (r *Registry) AddNorn(rw http.ResponseWriter, req *http.Request) {
	payload, err := ioutil.ReadAll(req.Body)

	norn, err := model.NewNorn(payload, req.RemoteAddr)
	if err != nil {
		jsonResponse(rw, http.StatusBadRequest, err.Error())
		return
	}

	nornPayload := map[string]interface{}{}
	data, err := json.Marshal(&norn)
	if err != nil {
		return
	}
	err = json.Unmarshal(data, &nornPayload)
	if err != nil {
		return
	}

	nornItem := argus.Item{
		Identifier: norn.DeviceID,
		Data:       nornPayload,
		TTL:        r.config.ArgusConfig.DefaultTTL,
	}
	_, err = r.hookStore.Push(nornItem, "")
	if err != nil {
		jsonResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(rw, http.StatusOK, "Success")
}

func (r *Registry) GetAllNorns(rw http.ResponseWriter, req *http.Request) (norns []model.Norn, err error) {
	items, err := r.hookStore.GetItems("")
	if err != nil {
		return
	}
	norns = []model.Norn{}
	for _, item := range items {
		norn, err := convertItemToNorn(item)
		if err != nil {
			log.WithPrefix(r.config.Logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Norn", "item", item)
			continue
		}
		norns = append(norns, norn)
	}
	return norns, nil
}

func (r *Registry) RemoveNorn(rw http.ResponseWriter, req *http.Request) (norns model.Norn, err error) {
	nornID := mux.Vars(req)
	id := nornID["id"]

	item, err := r.hookStore.Remove(id, "")
	if err != nil {
		// add loggging
	}
	norn, err := convertItemToNorn(item)
	if err != nil {
		// add logging
	}
	return norn, nil

}

func convertItemToNorn(item argus.Item) (model.Norn, error) {
	norn := model.Norn{}
	tempBytes, err := json.Marshal(&item.Data)
	if err != nil {
		return norn, err
	}
	err = json.Unmarshal(tempBytes, &norn)
	if err != nil {
		return norn, err
	}
	return norn, nil
}
