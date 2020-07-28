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

package main

import (
	"os"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/mimisbrunnr/dispatch"
	"github.com/xmidt-org/mimisbrunnr/eventParser"
	"github.com/xmidt-org/mimisbrunnr/manager"
	"github.com/xmidt-org/mimisbrunnr/registry"
	"github.com/xmidt-org/mimisbrunnr/routes"
	"go.uber.org/fx"
)

const (
	applicationName = "mimisbrunnr"
	apiBase         = "api/v1"
)

func main() {
	app := fx.New(

		fx.Provide(
			viper.New(),
			func(v *viper.Viper) (eventParser.Options, error) {
				var parserOpt eventParser.Options
				err := v.Unmarshal(&parserOpt)
				return parserOpt, err
			},

			func(v *viper.Viper) (dispatch.DispatcherConfig, error) {
				var dispatchConf dispatch.DispatcherConfig
				err := v.Unmarshal(&dispatchConf)
				return dispatchConf, err
			},

			manager.Provide,

			func(v *viper.Viper, m *manager.Manager) (registry.RegistryConfig, error) {
				var registryConf registry.RegistryConfig
				err := v.Unmarshal(&registryConf)
				registryConf.Listener = m.Update
				return registryConf, err
			},

			registry.NewRegistry,
			dispatch.ProvideMetrics,
			eventParser.Provide,

			routes.Provide,

			routes.ProvideServerChainFactory,
		),
		fx.Invoke(
			routes.BuildPrimaryRoutes,
			routes.BuildMetricsRoutes,
			routes.BuildHealthRoutes,
		),
	)
	switch err := app.Err(); err {
	case pflag.ErrHelp:
		return
	case nil:
		app.Run()
	default:
		os.Exit(2)
	}

}
