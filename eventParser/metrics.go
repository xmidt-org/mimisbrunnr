// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package eventParser

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/provider"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/themis/xmetrics"
	"go.uber.org/fx"
)

const (
	ParsingQueueDepth    = "parsing_queue_depth"
	DroppedEventsCounter = "dropped_events_count"
)

// ProvideMetrics is the metrics provider for eventParser
func ProvideMetrics() fx.Option {
	return fx.Provide(
		xmetrics.ProvideGauge(
			prometheus.GaugeOpts{
				Name: ParsingQueueDepth,
				Help: "The depth of the parsing queue",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: DroppedEventsCounter,
				Help: "The total number of events dropped"},
		),
	)
}

// Measures describes the defined metrics that will be used by eventparser
type Measures struct {
	fx.In
	EventParsingQueue         metrics.Gauge   `name:"parsing_queue_depth"`
	DroppedEventsParsingCount metrics.Counter `name:"dropped_events_count"`
}

// NewMeasures returns desired metrics
func NewMeasures(p provider.Provider) *Measures {
	return &Measures{
		EventParsingQueue:         p.NewGauge(ParsingQueueDepth),
		DroppedEventsParsingCount: p.NewCounter(DroppedEventsCounter),
	}
}
