package webhook

import (
	"os"
	"os/signal"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/argus/webhookclient"
	"github.com/xmidt-org/webpa-common/basculechecks"
	"github.com/xmidt-org/webpa-common/basculemetrics"
	"github.com/xmidt-org/webpa-common/concurrent"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/server"
	"github.com/xmidt-org/webpa-common/webhook"
)

const (
	applicationName = "mimisbrunnr"
)

type Config struct {
	ArgusConfig webhookclient.ClientConfig
}

func Mimisbrunnr() {
	beginMimisbrunnr := time.Now()

	var (
		f, v                                = pflag.NewFlagSet(applicationName, pflag.ContinueOnError), viper.New()
		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, os.Args, f, v, basculechecks.Metrics, basculemetrics.Metrics, Metrics)
	)

	var (
		infoLog = log.WithPrefix(logger, level.Key(), level.InfoValue())
	)

	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to initialize", logging.ErrorKey(), err.Error())
	}

	config := new(Config)
	err = v.Unmarshal(config)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to unmarshal config", logging.ErrorKey(), err.Error())
	}

	measures := NewMeasures(metricsRegistry)

	var updateListSizeMetric webhookclient.ListenerFunc
	updateListSizeMetric = func(hooks []webhook.W) {
		measures.WebhookListSize.Set(float64(len(hooks)))
	}

	webhookRegistry, err := NewRegistry(RegistryConfig{
		Logger:      logger,
		ArgusConfig: config.ArgusConfig,
	}, updateListSizeMetric)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "Unable to create new register", logging.ErrorKey(), err.Error())
	}

	primaryHandler, err := NewPrimaryHandler(logger, v, webhookRegistry)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "Validator error", logging.ErrorKey(), err.Error())
	}

	_, runnable, done := webPA.Prepare(logger, nil, metricsRegistry, primaryHandler)

	waitGroup, shutdown, err := concurrent.Execute(runnable)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "Unable to start device manager", logging.ErrorKey(), err.Error())
		os.Exit(1)
	}
	var messageKey = logging.MessageKey()

	infoLog.Log(messageKey, "Mimisbrunnr is up and running!", "elapsedTime", time.Since(beginMimisbrunnr))

	signals := make(chan os.Signal, 10)
	signal.Notify(signals, os.Kill, os.Interrupt)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "exiting due to signal", "signal", s)
			exit = true
		case <-done:
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "one or more servers exited")
			exit = true
		}
	}

	close(shutdown)
	waitGroup.Wait()

	return
}
