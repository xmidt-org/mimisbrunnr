// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

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
