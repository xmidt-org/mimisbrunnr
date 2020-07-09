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
	"github.com/xmidt-org/webpa-common/xmetrics"
)

const (
	EventQueueDepth   = "event_queue_depth"
	DroppedQueueCount = "dropped_queue_count"
	WorkersCount      = "workers_count"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name: EventQueueDepth,
			Help: "The depth of the event queue",
			Type: "gauge",
		},
		{
			Name: DroppedQueueCount,
			Help: "The total number of queues dropped from overflow",
			Type: "counter",
		},
		{
			Name: WorkersCount,
			Help: "The number of workers",
			Type: "gauge",
		},
	}
}

type Measures struct {
	EventQueueDepthGauge metrics.Gauge
	DroppedQueueCount    metrics.Counter
	WorkersCount         metrics.Gauge
}

func NewMeasures(p provider.Provider) *Measures {
	return &Measures{
		EventQueueDepthGauge: p.NewGauge(EventQueueDepth),
		DroppedQueueCount:    p.NewCounter(DroppedQueueCount),
		WorkersCount:         p.NewGauge(WorkersCount),
	}
}
