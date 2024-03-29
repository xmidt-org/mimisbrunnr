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

	"github.com/go-kit/log"
	"github.com/xmidt-org/argus/chrysom"
	"go.uber.org/fx"
)

// RegistryIn is the set of dependencies for this package's components
type RegistryIn struct {
	fx.In
	NornRegistry NornRegistry
}

type Registry struct {
	HookStore *chrysom.BasicClient
	Logger    log.Logger
}

type NornRegistry struct {
	Logger         log.Logger
	Listener       chrysom.ListenerFunc
	ListenerConfig chrysom.ListenerClientConfig
	Measures       chrysom.Measures
	Reader         chrysom.Reader
	Argus          chrysom.BasicClientConfig
}

func jsonResponse(rw http.ResponseWriter, code int, msg string) { //nolint: unused
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)
	rw.Write([]byte(fmt.Sprintf(`{"message":"%s"}`, msg)))
}

// NewRegistry returns Registry with configured argus client and listener
func NewRegistry(in RegistryIn) (*Registry, error) {

	argus, err := chrysom.NewBasicClient(in.NornRegistry.Argus, nil)
	if err != nil {
		return nil, err
	}

	return &Registry{
		Logger:    in.NornRegistry.Logger,
		HookStore: argus,
	}, nil
}
