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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	argus "github.com/xmidt-org/argus/model"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/mimisbrunnr/registry"
	"github.com/xmidt-org/webpa-common/v2/logging"
)

type BadRequestError struct {
	Request interface{}
}

func (bre BadRequestError) Error() string {
	return fmt.Sprintf("No value exists with request: %#v", bre)
}

func (bre BadRequestError) StatusCode() int {
	return http.StatusBadRequest
}

type IdOwnerItem struct {
	ID    string
	Owner string
	Item  argus.Item
}

// NewPostEndpoint returns the endpoint for /events handler
func NewPostEndpoint(r *registry.Registry) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			item argus.Item
			ok   bool
		)
		if item, ok = request.(argus.Item); !ok {
			return nil, BadRequestError{Request: request}
		}

		nornID, err := r.HookStore.Push(item, "")
		return nornID, err

	}
}

// NewEventsEndpointDecode returns DecodeRequestFunc wrapper for the /events endpoint
func NewPostEndpointDecode() kithttp.DecodeRequestFunc {
	return func(ctx context.Context, req *http.Request) (interface{}, error) {
		payload, err := ioutil.ReadAll(req.Body)

		nornReq, err := model.NewNornRequest(payload, req.RemoteAddr)
		if err != nil {
			return "", err
		}

		norn, err := model.NewNorn(nornReq, "")

		nornPayload := map[string]interface{}{}
		data, err := json.Marshal(&norn)
		if err != nil {
			return "", err
		}
		err = json.Unmarshal(data, &nornPayload)
		if err != nil {
			return "", err
		}

		nornItem := argus.Item{
			Identifier: nornReq.DeviceID,
			Data:       nornPayload,
			TTL:        norn.ExpiresAt,
		}
		return nornItem, nil
	}
}

// NewPutEndpointDecoder returns DecodeRequestFunc wrapper for the /norns/{id} endpoint
func NewPutEndpointDecoder() kithttp.DecodeRequestFunc {
	return func(ctx context.Context, req *http.Request) (interface{}, error) {
		nornID := mux.Vars(req)
		id := nornID["id"]

		payload, err := ioutil.ReadAll(req.Body)

		nornReq, err := model.NewNornRequest(payload, req.RemoteAddr)
		if err != nil {
			return "", err
		}

		norn, err := model.NewNorn(nornReq, "")

		nornPayload := map[string]interface{}{}
		data, err := json.Marshal(&norn)
		if err != nil {
			return "", err
		}
		err = json.Unmarshal(data, &nornPayload)
		if err != nil {
			return "", err
		}

		nornItem := argus.Item{
			Identifier: id,
			Data:       nornPayload,
			TTL:        norn.ExpiresAt,
		}
		return nornItem, nil
	}
}

// NewDeleteEndpoint returns the endpoint for /norns/{id} handler
func NewDeleteEndpoint(r *registry.Registry) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			idOwner IdOwnerItem
			ok      bool
		)
		if idOwner, ok = request.(IdOwnerItem); !ok {
			return nil, BadRequestError{Request: request}
		}
		if idOwner.ID == "" || idOwner.Owner == "" {
			return nil, BadRequestError{Request: request}
		}
		item, err := r.HookStore.Remove(idOwner.ID, idOwner.Owner)
		if err != nil {
			log.WithPrefix(r.Logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to remove item", "item", item)
		}
		norn, err := model.ConvertItemToNorn(item)
		if err != nil {
			log.WithPrefix(r.Logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Norn", "item", item)
		}
		return norn, nil
	}

}

// NewDeleteEndpointDecode returns DecodeRequestFunc wrapper for the /norns/{id} endpoint
func NewDeleteEndpointDecode() kithttp.DecodeRequestFunc {
	return func(ctx context.Context, req *http.Request) (interface{}, error) {
		nornID := mux.Vars(req)
		id := nornID["id"]
		owner := ""

		return &IdOwnerItem{
			ID:    id,
			Owner: owner,
		}, nil
	}
}

// NewGetAllEndpoint returns the endpoint for /norns handler
func NewGetAllEndpoint(r *registry.Registry) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			norns   []model.Norn
			idOwner IdOwnerItem
			ok      bool
		)
		if idOwner, ok = request.(IdOwnerItem); !ok {
			return nil, BadRequestError{Request: request}
		}

		items, err := r.HookStore.GetItems(idOwner.Owner)
		if err != nil {
			return norns, err
		}

		for _, item := range items {
			norn, err := model.ConvertItemToNorn(item)
			if err != nil {
				log.WithPrefix(r.Logger, level.Key(), level.ErrorValue()).Log(logging.MessageKey(), "failed to convert Item to Norn", "item", item)
				continue
			}
			norns = append(norns, norn)
		}
		return norns, nil
	}
}

// NewGetAllEndpointDecode returns DecodeRequestFunc wrapper for the /norns endpoint
func NewGetAllEndpointDecode() kithttp.DecodeRequestFunc {
	return func(ctx context.Context, req *http.Request) (interface{}, error) {
		// use bascule stuff here for owner
		owner := ""
		return &IdOwnerItem{
			Owner: owner,
		}, nil
	}
}

// NewSetEndpointEncode returns EncodeResponseFunc wrapper for the /norns endpoint
func NewSetEndpointEncode() kithttp.EncodeResponseFunc {
	return func(ctx context.Context, resp http.ResponseWriter, value interface{}) error {
		if value != nil {
			data, err := json.Marshal(&value)
			if err != nil {
				return err
			}
			resp.Header().Add("Content-Type", "application/json")
			resp.Write(data)
		}
		return nil
	}
}
