// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package routes

import (
	"net/http"

	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/xmidt-org/mimisbrunnr/eventParser"
	"github.com/xmidt-org/mimisbrunnr/manager"
	"github.com/xmidt-org/mimisbrunnr/norn"
	"github.com/xmidt-org/mimisbrunnr/registry"
	"go.uber.org/fx"
)

// HandlerIn is the set of dependencies for this package's components
type HandlerIn struct {
	fx.In
	Registry    *registry.Registry
	Manager     *manager.Manager
	EventParser *eventParser.EventParser
}

// HandlerOut is the set of components emitted by this package
type HandlerOut struct {
	fx.Out

	PostKeyHandler http.Handler `name:"postHandler"`

	GetKeyHandler http.Handler `name:"getHandler"`

	DeleteKeyHandler http.Handler `name:"deleteHandler"`

	GetAllKeyHandler http.Handler `name:"getAllHandler"`

	PutKeyHandler http.Handler `name:"putHandler"`

	EventsKeyHandler http.Handler `name:"eventHandler"`
}

// Provide is an uber/fx style provider for this package's components
func Provide(in HandlerIn) HandlerOut {
	return HandlerOut{
		PostKeyHandler:   kithttp.NewServer(norn.NewPostEndpoint(in.Registry), norn.NewPostEndpointDecode(), norn.NewSetEndpointEncode()),
		GetAllKeyHandler: kithttp.NewServer(norn.NewGetAllEndpoint(in.Registry), norn.NewGetAllEndpointDecode(), norn.NewSetEndpointEncode()),
		DeleteKeyHandler: kithttp.NewServer(norn.NewDeleteEndpoint(in.Registry), norn.NewDeleteEndpointDecode(), norn.NewSetEndpointEncode()),
		GetKeyHandler:    kithttp.NewServer(manager.NewGetEndpoint(in.Manager), manager.NewGetEndpointDecode(), norn.NewSetEndpointEncode()),
		PutKeyHandler:    kithttp.NewServer(norn.NewPostEndpoint(in.Registry), norn.NewPutEndpointDecoder(), norn.NewSetEndpointEncode()),
		EventsKeyHandler: kithttp.NewServer(eventParser.NewEventsEndpoint(in.EventParser), eventParser.NewEventsEndpointDecode(), eventParser.NewEventsEndpointEncode()),
	}
}
