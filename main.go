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
	"github.com/xmidt-org/mimisbrunnr/eventParser"
	"go.uber.org/fx"
)

const (
	applicationName = "mimisbrunnr"
	apiBase         = "norns"
)

type Config struct {
	internalQueue eventParser.InternalQueue
	// add the endpoints
}

func main() {
	app := fx.New(
		fx.Provide(
			viper.New(),

			func(v *viper.Viper) (Config, error) {
				var config Config
				err := v.Unmarshal(&config)
				return config, err
			},
			// Provide Queue
			// eventregistration.Listener,
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
