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

package manager

import (
	"github.com/xmidt-org/argus/chrysom"
	"github.com/xmidt-org/mimisbrunnr/dispatch"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/wrp-go/v2"
)

type Manager interface {
	SendEvent(wrp.Message, map[*model.Destination]dispatch.D)
}

type Manage struct {
	listener chrysom.Listener
	manager  Manager
}

func ProvideDestinationMap(norns []model.Norn) map[*model.Destination]dispatch.D {
	var destinationMap map[*model.Destination]dispatch.D
	for _, norn := range norns {
		destinationMap[&norn.Destination]
	}

}
