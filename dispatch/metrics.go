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

package dispatch

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/provider"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/themis/xmetrics"
	"go.uber.org/fx"
)

// Dispatcher metrics
const (
	EventQueueDepth               = "event_queue_depth"
	DroppedQueueCount             = "dropped_queue_count"
	WorkersCount                  = "workers_count"
	ConsumerDropUntilGauge        = "consumer_drop_until"
	SlowConsumerCounter           = "slow_consumer_cut_off_count"
	SlowConsumerDroppedMsgCounter = "slow_consumer_dropped_message_count"
	IncomingContentTypeCounter    = "incoming_content_type_count"
	ConsumerDeliverUntilGauge     = "consumer_deliver_until"
	DeliveryCounter               = "delivery_count"
	DroppedPanic                  = "dropped_panic_count"
	DroppedNetworkErrCounter      = "dropped_network_error_count"
	DroppedInvalidConfig          = "dropped_invalid_config"
	DroppedExpiredCounter         = "dropped_expired_count"
)

// ProvideMetrics returns the Measures relevant to dispatch package
func ProvideMetrics() fx.Option {
	return fx.Provide(
		xmetrics.ProvideGauge(
			prometheus.GaugeOpts{
				Name: EventQueueDepth,
				Help: "The depth of the event queue",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: DroppedQueueCount,
				Help: "The total number of queues dropped from overflow",
			},
		),
		xmetrics.ProvideGauge(
			prometheus.GaugeOpts{
				Name: WorkersCount,
				Help: "The number of workers",
			},
		),
		xmetrics.ProvideGauge(
			prometheus.GaugeOpts{
				Name: ConsumerDropUntilGauge,
				Help: "The time after which events going to a customer will be delivered.",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: SlowConsumerCounter,
				Help: "Count of the number of times a consumer has been deemed too slow and is cut off.",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: SlowConsumerDroppedMsgCounter,
				Help: "Count of dropped messages due to a slow consumer.",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: IncomingContentTypeCounter,
				Help: "Count of the content type processed.",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: DeliveryCounter,
				Help: "Count of delivered messages to a url with a status code",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: DroppedPanic,
				Help: "Count of dropped messages due to panic",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: DroppedNetworkErrCounter,
				Help: "Count of dropped messages due to network error",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: DroppedInvalidConfig,
				Help: "Count of dropped messages due to invalid configuration",
			},
		),
		xmetrics.ProvideCounter(
			prometheus.CounterOpts{
				Name: DroppedExpiredCounter,
				Help: "Count of dropped messages due to expired norn",
			},
		),
	)
}

// Measures describes the defined metrics that will be used by dispatcher
type Measures struct {
	fx.In
	EventQueueDepthGauge     metrics.Gauge   `name:"event_queue_depth"`
	DroppedQueueCount        metrics.Counter `name:"dropped_queue_count"`
	WorkersCount             metrics.Gauge   `name:"workers_count"`
	DropUntilGauge           metrics.Gauge   `name:"consumer_drop_until"`
	CutOffCounter            metrics.Counter `name:"slow_consumer_cut_off_count"`
	DroppedCutoffCounter     metrics.Counter `name:"slow_consumer_dropped_message_count"`
	ContentTypeCounter       metrics.Counter `name:"incoming_content_type_count"`
	DroppedNetworkErrCounter metrics.Counter `name:"dropped_network_error_count"`
	DeliveryCounter          metrics.Counter `name:"delivery_count"`
	DroppedInvalidConfig     metrics.Counter `name:"dropped_invalid_config"`
	DroppedExpiredCounter    metrics.Counter `name:"dropped_expired_count"`
	DroppedPanicCounter      metrics.Counter `name:"dropped_panic_count"`
}

// NewMeasures returns desired metrics
func NewMeasures(p provider.Provider) *Measures {
	return &Measures{
		EventQueueDepthGauge:     p.NewGauge(EventQueueDepth),
		DroppedQueueCount:        p.NewCounter(DroppedQueueCount),
		WorkersCount:             p.NewGauge(WorkersCount),
		DropUntilGauge:           p.NewGauge(ConsumerDropUntilGauge),
		CutOffCounter:            p.NewCounter(SlowConsumerCounter),
		DroppedCutoffCounter:     p.NewCounter(SlowConsumerDroppedMsgCounter),
		ContentTypeCounter:       p.NewCounter(IncomingContentTypeCounter),
		DroppedNetworkErrCounter: p.NewCounter(SlowConsumerDroppedMsgCounter),
		DeliveryCounter:          p.NewCounter(DeliveryCounter),
		DroppedInvalidConfig:     p.NewCounter(SlowConsumerDroppedMsgCounter),
		DroppedExpiredCounter:    p.NewCounter(SlowConsumerDroppedMsgCounter),
		DroppedPanicCounter:      p.NewCounter(DroppedPanic),
	}
}
