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

package registry

import (
	"fmt"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/argus/chrysom"
	"go.uber.org/fx"
)

type RegistryIn struct {
	fx.In
	RegistryConfig RegistryConfig
}

type Registry struct {
	HookStore *chrysom.Client
	Logger    log.Logger
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

func NewRegistry(in RegistryIn) (*Registry, error) {

	argus, err := chrysom.CreateClient(in.RegistryConfig.ArgusConfig, chrysom.WithLogger(in.RegistryConfig.Logger))
	if err != nil {
		return nil, err
	}
	if in.RegistryConfig.Listener != nil {
		argus.SetListener(in.RegistryConfig.Listener)
	}
	return &Registry{
		Logger:    in.RegistryConfig.Logger,
		HookStore: argus,
	}, nil
}
