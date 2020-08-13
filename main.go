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
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/InVisionApp/go-health"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/mimisbrunnr/dispatch"
	"github.com/xmidt-org/mimisbrunnr/eventParser"
	"github.com/xmidt-org/mimisbrunnr/manager"
	"github.com/xmidt-org/mimisbrunnr/registry"
	"github.com/xmidt-org/mimisbrunnr/routes"
	"github.com/xmidt-org/themis/config"
	"github.com/xmidt-org/themis/xhealth"
	"github.com/xmidt-org/themis/xhttp/xhttpserver"
	"github.com/xmidt-org/themis/xlog"
	"github.com/xmidt-org/themis/xmetrics/xmetricshttp"
	"go.uber.org/fx"
)

const (
	applicationName = "mimisbrunnr"
	DefaultKeyID    = "current"
	apiBase         = "api/v1"
)

var (
	GitCommit = "undefined"
	Version   = "undefined"
	BuildTime = "undefined"
)

func setupFlagSet(fs *pflag.FlagSet) error {
	fs.StringP("file", "f", "", "the configuration file to use.  Overrides the search path.")
	fs.BoolP("debug", "d", false, "enables debug logging.  Overrides configuration.")
	fs.BoolP("version", "v", false, "print version and exit")

	return nil
}

func setupViper(in config.ViperIn, v *viper.Viper) (err error) {
	if printVersion, _ := in.FlagSet.GetBool("version"); printVersion {
		printVersionInfo()
	}
	if file, _ := in.FlagSet.GetString("file"); len(file) > 0 {
		v.SetConfigFile(file)
		err = v.ReadInConfig()
	} else {
		v.SetConfigName(string(in.Name))
		v.AddConfigPath(fmt.Sprintf("/etc/%s", in.Name))
		v.AddConfigPath(fmt.Sprintf("$HOME/.%s", in.Name))
		v.AddConfigPath(".")
		err = v.ReadInConfig()
	}

	if err != nil {
		return err
	}

	if debug, _ := in.FlagSet.GetBool("debug"); debug {
		v.Set("log.level", "DEBUG")
	}

	return nil
}

func printVersionInfo() {
	fmt.Fprintf(os.Stdout, "%s:\n", applicationName)
	fmt.Fprintf(os.Stdout, "  version: \t%s\n", Version)
	fmt.Fprintf(os.Stdout, "  go version: \t%s\n", runtime.Version())
	fmt.Fprintf(os.Stdout, "  built time: \t%s\n", BuildTime)
	fmt.Fprintf(os.Stdout, "  git commit: \t%s\n", GitCommit)
	fmt.Fprintf(os.Stdout, "  os/arch: \t%s/%s\n", runtime.GOOS, runtime.GOARCH)
	os.Exit(0)
}

func main() {
	app := fx.New(
		xlog.Logger(),
		config.CommandLine{Name: applicationName}.Provide(setupFlagSet),
		dispatch.ProvideMetrics(),
		eventParser.ProvideMetrics(),
		provideMetrics(),
		fx.Provide(
			config.ProvideViper(setupViper),
			xlog.Unmarshal("log"),
			func(v *viper.Viper) (eventParser.ParserConfig, error) {
				config := new(eventParser.ParserConfig)
				err := v.UnmarshalKey("parserConfig", &config)
				return *config, err
			},
			func(v *viper.Viper) (dispatch.DispatcherConfig, error) {
				config := new(dispatch.DispatcherConfig)
				err := v.UnmarshalKey("dispatcherConfig", &config)
				return *config, err
			},
			func(v *viper.Viper) (dispatch.FilterConfig, error) {
				config := new(dispatch.FilterConfig)
				err := v.UnmarshalKey("filterConfig", &config)
				return *config, err
			},
			manager.Provide,
			func(v *viper.Viper, m *manager.Manager) (registry.RegistryConfig, error) {
				config := new(registry.RegistryConfig)
				err := v.UnmarshalKey("registryConfig", &config)
				config.Listener = m.Update
				return *config, err
			},
			func(m *manager.Manager) eventParser.EventSenderFunc {
				return m.Send
			},
			registry.NewRegistry,
			eventParser.Provide,
			routes.Provide,
			routes.ProvideServerChainFactory,
			xmetricshttp.Unmarshal("prometheus", promhttp.HandlerOpts{}),
			xhealth.Unmarshal("health"),
			xhttpserver.Unmarshal{Key: "servers.primary", Optional: true}.Annotated(),
			xhttpserver.Unmarshal{Key: "servers.metrics", Optional: true}.Annotated(),
			xhttpserver.Unmarshal{Key: "servers.health", Optional: true}.Annotated(),
		),
		fx.Invoke(
			xhealth.ApplyChecks(
				&health.Config{
					Name:     applicationName,
					Interval: 24 * time.Hour,
					Checker: xhealth.NopCheckable{
						Details: map[string]interface{}{
							"StartTime": time.Now().UTC().Format(time.RFC3339),
						},
					},
				},
			),
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
