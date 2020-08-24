/**
 * Copyright 2019 Comcast Cable Communications Management, LLC
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

type Measures struct {
	fx.In
	EventParsingQueue         metrics.Gauge   `name:"parsing_queue_depth"`
	DroppedEventsParsingCount metrics.Counter `name:"dropped_events_count"`
}

func NewMeasures(p provider.Provider) *Measures {
	return &Measures{
		EventParsingQueue:         p.NewGauge(ParsingQueueDepth),
		DroppedEventsParsingCount: p.NewCounter(DroppedEventsCounter),
	}
}
