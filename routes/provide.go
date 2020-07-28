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

	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/xmidt-org/mimisbrunnr/eventParser"
	"github.com/xmidt-org/mimisbrunnr/manager"
	"github.com/xmidt-org/mimisbrunnr/norn"
	"github.com/xmidt-org/mimisbrunnr/registry"
	"go.uber.org/fx"
)

type HandlerIn struct {
	fx.In
	Registry    *registry.Registry
	Manager     *manager.Manager
	EventParser *eventParser.EventParser
}

type HandlerOut struct {
	fx.Out

	PostKeyHandler http.Handler `name:"postHandler"`

	GetKeyHandler http.Handler `name:"getHandler"`

	DeleteKeyHandler http.Handler `name:"deleteHandler"`

	GetAllKeyHandler http.Handler `name:"getAllHandler"`

	PutKeyHandler http.Handler `name:"putHandler"`

	EventsKeyHandler http.Handler `name:"eventHandler"`
}

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
