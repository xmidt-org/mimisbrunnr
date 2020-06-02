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

package eventregistration

import (
	"crypto/sha1"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/bascule/acquire"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/webpa-common/basculechecks"
	"github.com/xmidt-org/webpa-common/basculemetrics"
	"github.com/xmidt-org/webpa-common/concurrent"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/server"
	webhook "github.com/xmidt-org/wrp-listener"
	"github.com/xmidt-org/wrp-listener/hashTokenFactory"
	secretGetter "github.com/xmidt-org/wrp-listener/secret"
	"github.com/xmidt-org/wrp-listener/webhookClient"
)

const (
	applicationName = "mimisbrunnr"
	TimeInMemory    = "queue_empty_duration"
)

type Config struct {
	Webhook Webhook
	Secret  Secret
}

type Webhook struct {
	RegistrationInterval time.Duration
	Timeout              time.Duration
	RegistrationURL      string
	Request              Request
	Basic                string
	JWT                  JWT
}

type Request struct {
	WebhookConfig WebhookConfig
	Events        string
}

type WebhookConfig struct {
	URL           string
	FailureURL    string
	Secret        string
	MaxRetryCount int
}

type Secret struct {
	Header    string
	Delimiter string
}

type JWT struct {
	RequestHeaders map[string]string
	AuthURL        string
	Timeout        time.Duration
	Buffer         time.Duration
}

func Listener() {

	var (
		f, v                                     = pflag.NewFlagSet(applicationName, pflag.ContinueOnError), viper.New()
		logger, metricsRegistry, caduceator, err = server.Initialize(applicationName, os.Args, f, v, basculechecks.Metrics, basculemetrics.Metrics, Metrics)
	)

	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to initialize", logging.ErrorKey(), err.Error())
	}

	config := new(Config)
	err = v.Unmarshal(config)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to unmarshal config", logging.ErrorKey(), err.Error())
	}

	// use constant secret for hash
	secretGetter := secretGetter.NewConstantSecret(config.Webhook.Request.WebhookConfig.Secret)

	// set up the middleware
	htf, err := hashTokenFactory.New("Sha1", sha1.New, secretGetter)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to setup hash token factory", logging.ErrorKey(), err.Error())
		os.Exit(1)
	}
	authConstructor := basculehttp.NewConstructor(
		basculehttp.WithTokenFactory("Sha1", htf),
		basculehttp.WithHeaderName(config.Secret.Header),
		basculehttp.WithHeaderDelimiter(config.Secret.Delimiter),
	)
	eventHandler := alice.New(authConstructor)

	var acquirer *acquire.RemoteBearerTokenAcquirer

	var webhookURLs []string

	// set up the registerer
	basicConfig := webhookClient.BasicConfig{
		Timeout:         config.Webhook.Timeout,
		RegistrationURL: config.Webhook.RegistrationURL,
		Request: webhook.W{
			Config: webhook.Config{
				URL: config.Webhook.Request.WebhookConfig.URL,
			},
			Events:     []string{config.Webhook.Request.Events},
			FailureURL: config.Webhook.Request.WebhookConfig.FailureURL,
		},
	}

	webhookURLs = append(webhookURLs, basicConfig.Request.Config.URL)

	acquireConfig := acquire.RemoteBearerTokenAcquirerOptions{
		AuthURL:        config.Webhook.JWT.AuthURL,
		Timeout:        config.Webhook.JWT.Timeout,
		Buffer:         config.Webhook.JWT.Buffer,
		RequestHeaders: config.Webhook.JWT.RequestHeaders,
	}

	acquirer, err = acquire.NewRemoteBearerTokenAcquirer(acquireConfig)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to create bearer auth plain text acquirer:", logging.ErrorKey(), err.Error())
		os.Exit(1)
	}

	registerer, err := webhookClient.NewBasicRegisterer(acquirer, secretGetter, basicConfig)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to setup registerer", logging.ErrorKey(), err.Error())
		os.Exit(1)
	}

	periodicRegisterer := webhookClient.NewPeriodicRegisterer(registerer, config.Webhook.RegistrationInterval, logger)

	periodicRegisterer.Start()

	router := mux.NewRouter()

	app := &App{logger: logger}

	// start listening
	logging.Info(logger).Log(logging.MessageKey(), "before handler")

	router.Handle("/events", eventHandler.ThenFunc(app.receiveEvents)).Methods("POST")

	logging.Info(logger).Log(logging.MessageKey(), "after handler")

	_, runnable, done := caduceator.Prepare(logger, nil, metricsRegistry, router)
	waitGroup, shutdown, err := concurrent.Execute(runnable)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to execute additional process", logging.ErrorKey(), err.Error())
		os.Exit(1)
	}

	signals := make(chan os.Signal, 10)
	signal.Notify(signals)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			if s != os.Kill && s != os.Interrupt {
				logging.Info(logger).Log(logging.MessageKey(), "ignoring signal", "signal", s)
			} else {
				logging.Error(logger).Log(logging.MessageKey(), "exiting due to signal", "signal", s)
				exit = true
			}
		case <-done:
			logging.Error(logger).Log(logging.MessageKey(), "one or more servers exited")
			exit = true
		}
	}

	periodicRegisterer.Stop()
	close(shutdown)
	waitGroup.Wait()
	logging.Info(logger).Log(logging.MessageKey(), "Mimisbrunnr has shut down")

}
