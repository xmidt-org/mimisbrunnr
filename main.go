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
	"github.com/xmidt-org/mimisbrunnr/norn"
	"go.uber.org/fx"
)

const (
	applicationName = "mimisbrunnr"
	apiBase         = "norns"
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

			func(v *viper.Viper) (norn.RegistryConfig, error) {
				var registryConf norn.RegistryConfig
				err := v.Unmarshal(&registryConf)
				return registryConf, err
			},

			norn.NewRegistry,
			dispatch.ProvideMetrics,
			eventParser.Provide,

			viper.New(),
		),
		fx.Invoke(
			BuildPrimaryRoutes,
			BuildMetricsRoutes,
			BuildHealthRoutes,
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
