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
