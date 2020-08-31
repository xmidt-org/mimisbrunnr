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

package routes

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/themis/xhealth"
	"github.com/xmidt-org/themis/xhttp/xhttpserver"
	"github.com/xmidt-org/themis/xmetrics"
	"github.com/xmidt-org/themis/xmetrics/xmetricshttp"
	"go.uber.org/fx"
)

var (
	ServerLabel = "Mimisbrunnr"
)

const (
	applicationName = "mimisbrunnr"
	apiBase         = "api/v1"
)

// ServerChainIn is the set of dependencies for this package's components
type ServerChainIn struct {
	fx.In

	RequestCount     *prometheus.CounterVec   `name:"server_request_count"`
	RequestDuration  *prometheus.HistogramVec `name:"server_request_duration_ms"`
	RequestsInFlight *prometheus.GaugeVec     `name:"server_requests_in_flight"`

	AuthChain *alice.Chain `name:"auth_chain"`
}

// ProvideServerChainFactory returns ChainFactory needed to set up primary server
func ProvideServerChainFactory(in ServerChainIn) xhttpserver.ChainFactory {
	return xhttpserver.ChainFactoryFunc(func(name string, o xhttpserver.Options) (alice.Chain, error) {
		var (
			curryLabel = prometheus.Labels{
				ServerLabel: name,
			}

			serverLabellers = xmetricshttp.NewServerLabellers(
				xmetricshttp.CodeLabeller{},
				xmetricshttp.MethodLabeller{},
			)
		)

		requestCount, err := in.RequestCount.CurryWith(curryLabel)
		if err != nil {
			return alice.Chain{}, err
		}

		requestDuration, err := in.RequestDuration.CurryWith(curryLabel)
		if err != nil {
			return alice.Chain{}, err
		}

		requestsInFlight, err := in.RequestsInFlight.CurryWith(curryLabel)
		if err != nil {
			return alice.Chain{}, err
		}

		if name == "servers.primary" {
			return in.AuthChain.Append(
				xmetricshttp.HandlerCounter{
					Metric:   xmetrics.LabelledCounterVec{CounterVec: requestCount},
					Labeller: serverLabellers,
				}.Then,
				xmetricshttp.HandlerDuration{
					Metric:   xmetrics.LabelledObserverVec{ObserverVec: requestDuration},
					Labeller: serverLabellers,
				}.Then,
				xmetricshttp.HandlerInFlight{
					Metric: xmetrics.LabelledGaugeVec{GaugeVec: requestsInFlight},
				}.Then,
			), nil
		}
		return alice.New(
			xmetricshttp.HandlerCounter{
				Metric:   xmetrics.LabelledCounterVec{CounterVec: requestCount},
				Labeller: serverLabellers,
			}.Then,
			xmetricshttp.HandlerDuration{
				Metric:   xmetrics.LabelledObserverVec{ObserverVec: requestDuration},
				Labeller: serverLabellers,
			}.Then,
			xmetricshttp.HandlerInFlight{
				Metric: xmetrics.LabelledGaugeVec{GaugeVec: requestsInFlight},
			}.Then,
		), nil
	})
}

type PrimaryRouter struct {
	fx.In
	Router  *mux.Router  `name:"servers.primary"`
	Handler http.Handler `name:"postHandler"`
}

type PostRoutesIn struct {
	fx.In
	Handler http.Handler `name:"postHandler"`
}
type GetRoutesIn struct {
	fx.In
	Handler http.Handler `name:"getHandler"`
}
type DeleteRoutesIn struct {
	fx.In
	Handler http.Handler `name:"deleteHandler"`
}

type PutRoutesIn struct {
	fx.In
	Handler http.Handler `name:"putHandler"`
}

type GetAllRoutesIn struct {
	fx.In
	Handler http.Handler `name:"getAllHandler"`
}

type PostEventRouteIn struct {
	fx.In
	Handler http.Handler `name:"eventHandler"`
}

// BuildPrimaryRoutes sets up all endpoint handlers for application
func BuildPrimaryRoutes(router PrimaryRouter, pin PostRoutesIn, gin GetRoutesIn, din DeleteRoutesIn, puin PutRoutesIn, gain GetAllRoutesIn, pein PostEventRouteIn) {
	if router.Handler != nil {
		if pin.Handler != nil {
			router.Router.Handle("/norns", pin.Handler).Methods("POST")
		}
		if gin.Handler != nil {
			router.Router.Handle("/norns/{id}", gin.Handler).Methods("GET")
		}
		if din.Handler != nil {
			router.Router.Handle("/norns/{id}", din.Handler).Methods("DELETE")
		}
		if puin.Handler != nil {
			router.Router.Handle("/norns/{id}", puin.Handler).Methods("PUT")
		}
		if gain.Handler != nil {
			router.Router.Handle("/norns", gain.Handler).Methods("GET")
		}
		if pein.Handler != nil {
			router.Router.Handle("/events", pein.Handler).Methods("POST")
		}
	}
}

type MetricsRoutesIn struct {
	fx.In
	Router  *mux.Router `name:"servers.metrics"`
	Handler xmetricshttp.Handler
}

// BuildMetricsRoutes sets up metrics handler
func BuildMetricsRoutes(in MetricsRoutesIn) {
	if in.Router != nil && in.Handler != nil {
		in.Router.Handle("/metrics", in.Handler).Methods("GET")
	}
}

type HealthRoutesIn struct {
	fx.In
	Router  *mux.Router `name:"servers.health"`
	Handler xhealth.Handler
}

// BuildHealthRoutes sets up health handler
func BuildHealthRoutes(in HealthRoutesIn) {
	if in.Router != nil && in.Handler != nil {
		in.Router.Handle("/health", in.Handler).Methods("GET")
	}
}
