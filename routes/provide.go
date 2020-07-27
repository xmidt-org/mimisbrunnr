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

	"github.com/xmidt-org/mimisbrunnr/eventParser"
	"github.com/xmidt-org/mimisbrunnr/manager"
	"github.com/xmidt-org/mimisbrunnr/norn"
	"go.uber.org/fx"
)

type HandlerIn struct {
	fx.In
	Registry    *norn.Registry
	Manager     *manager.Manager
	EventParser *eventParser.EventParser
}

type HandlerOut struct {
	fx.Out

	SetKeyHandler http.Handler `name:"setHandler"`

	GetKeyHandler http.Handler `name:"getHandler"`

	DeleteKeyHandler http.Handler `name:"deleteHandler"`

	GetAllKeyHandler http.Handler `name:"getAllHandler"`

	EventsKeyHandler http.Handler `name:"eventHandler"`
}

func Provide(in HandlerIn) HandlerOut {
	return HandlerOut{
		SetKeyHandler:    norn.NewHandler(norn.NewPostEndpoint(in.Registry), norn.NewPostEndpointDecode(), norn.NewSetEndpointEncode()),
		GetAllKeyHandler: norn.NewHandler(norn.NewGetAllEndpoint(in.Registry), norn.NewGetAllEndpointDecode(), norn.NewSetEndpointEncode()),
		DeleteKeyHandler: norn.NewHandler(norn.NewDeleteEndpoint(in.Registry), norn.NewDeleteEndpointDecode(), norn.NewSetEndpointEncode()),
		GetKeyHandler:    norn.NewHandler(manager.NewGetEndpoint(in.Manager), manager.NewGetEndpointDecode(), norn.NewSetEndpointEncode()),
		EventsKeyHandler: norn.NewHandler(eventParser.NewEventsEndpoint(in.EventParser), eventParser.NewEventsEndpointDecode(), eventParser.NewEventsEndpointEncode()),
	}
}
