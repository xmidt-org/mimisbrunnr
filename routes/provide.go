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
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/mimisbrunnr/norn"
	"go.uber.org/fx"
)

type HandlerIn struct {
	fx.In

	Registry *norn.Registry

	Manager *manager.Manager

	EventParser *eventParser.EventParser
}

type HandlerOut struct {
	fx.Out

	SetKeyHandler func(http.ResponseWriter, *http.Request) (string, int) `name:"setHandler"`

	GetKeyHandler func(rw http.ResponseWriter, req *http.Request) (model.Norn, int) `name:"getHandler"`

	DeleteKeyHandler func(rw http.ResponseWriter, req *http.Request) (model.Norn, error) `name:"deleteHandler"`

	GetAllKeyHandler func(rw http.ResponseWriter, req *http.Request) ([]model.Norn, int) `name:"getAllHandler"`

	EventsKeyHandler func(writer http.ResponseWriter, req *http.Request) `name:"eventHandler"`
}

func Provide(in HandlerIn) HandlerOut {
	return HandlerOut{
		SetKeyHandler:    in.Registry.AddNorn,
		GetKeyHandler:    in.Manager.GetNorn,
		DeleteKeyHandler: in.Registry.RemoveNorn,
		GetAllKeyHandler: in.Registry.GetAllNorns,
		EventsKeyHandler: in.EventParser.HandleEvents,
	}
}
