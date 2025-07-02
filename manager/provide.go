// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"net/http"

	"github.com/go-kit/log"
	"github.com/xmidt-org/mimisbrunnr/dispatch"
	"go.uber.org/fx"
)

// ManagerIn is the set of dependencies for this package's components
type ManagerIn struct {
	fx.In

	DispatcherConfig dispatch.SenderConfig
	// FilterConfig     dispatch.FilterConfig
	Logger    log.Logger
	Measures  dispatch.Measures
	Transport http.RoundTripper
}

// Provide is an uber/fx style provider for this package's components
func Provide(in ManagerIn) (*Manager, error) {
	m, err := NewManager(in.DispatcherConfig, in.Transport, in.Logger, in.Measures)
	if err != nil {
		return nil, err
	}

	return m, nil
}
