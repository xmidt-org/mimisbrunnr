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
	"fmt"
	"net/http"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/xmidt-org/argus/chrysom"
)

type Registry struct {
	hookStore *chrysom.Client
	logger    log.Logger
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

func NewRegistry(config RegistryConfig) (*Registry, error) {
	argus, err := chrysom.CreateClient(config.ArgusConfig, chrysom.WithLogger(config.Logger))
	if err != nil {
		return nil, err
	}
	if config.Listener != nil {
		argus.SetListener(config.Listener)
	} else {
		config.Logger.Log("Chrysom Listener not set.")
	}
	return &Registry{
		logger:    config.Logger,
		hookStore: argus,
	}, nil
}

func NewHandler(endpoint endpoint.Endpoint, decode kithttp.DecodeRequestFunc, encode kithttp.EncodeResponseFunc) http.Handler {
	return kithttp.NewServer(endpoint, decode, encode)
}
