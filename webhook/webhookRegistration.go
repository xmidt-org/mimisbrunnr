package webhook

import (
	"os"
	"os/signal"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
}

func Mimisbrunnr() {
	beginMimisbrunnr := time.Now()

	var (
		f, v                                = pflag.NewFlagSet(applicationName, pflag.ContinueOnError), viper.New()
		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, os.Args, f, v, basculechecks.Metrics, basculemetrics.Metrics, Metrics)
	)

	var (
		infoLog  = log.WithPrefix(logger, level.Key(), level.InfoValue())
		errorLog = log.WithPrefix(logger, level.Key(), level.ErrorValue())
		debugLog = log.WithPrefix(logger, level.Key(), level.DebugValue())
	)

	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to initialize", logging.ErrorKey(), err.Error())
	}

	config := new(Config)
	err = v.Unmarshal(config)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "failed to unmarshal config", logging.ErrorKey(), err.Error())
	}

	webhookFactory, err := webhook.NewFactory(v)
	if err != nil {
		logging.Error(logger).Log(logging.MessageKey(), "Error creating new webhook factory", logging.ErrorKey(), err.Error())
	}
	webhookRegistry, _ := webhookFactory.NewRegistryAndHandler(metricsRegistry)
	primaryHandler, err := NewPrimaryHandler(logger, v, &webhookRegistry)
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

	if webhookFactory != nil {
		// wait for DNS to propagate before subscribing to SNS
		if err = webhookFactory.DnsReady(); err == nil {
			debugLog.Log(messageKey, "Calling webhookFactory.PrepareAndStart. Server is ready to take on subscription confirmations")
			webhookFactory.PrepareAndStart()
		} else {
			errorLog.Log(messageKey, "Server was not ready within a time constraint. SNS confirmation could not happen",
				logging.ErrorKey(), err)
		}
	}

	// Attempt to obtain the current listener list from current system without having to wait for listener reregistration.
	debugLog.Log(messageKey, "Attempting to obtain current listener list from source", "source",
		v.GetString("start.apiPath"))
	beginObtainList := time.Now()
	startChan := make(chan webhook.Result, 1)
	webhookFactory.Start.GetCurrentSystemsHooks(startChan)
	var webhookStartResults webhook.Result = <-startChan
	if webhookStartResults.Error != nil {
		errorLog.Log(logging.ErrorKey(), webhookStartResults.Error)
	} else {
		// todo: add message
		webhookFactory.SetList(webhook.NewList(webhookStartResults.Hooks))
		// todo: add internal queing system for mimisbrunnr
		// caduceusSenderWrapper.Update(webhookStartResults.Hooks)
	}

	debugLog.Log(messageKey, "Current listener retrieval.", "elapsedTime", time.Since(beginObtainList))
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

	// shutdown the sender wrapper gently so that all queued messages get serviced
	// caduceusSenderWrapper.Shutdown(true)
	return
}
