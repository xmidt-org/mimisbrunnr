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
	"context"

	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/wrp-go/v2"
)

type D interface {
	Start(context.Context) error
	Send(*wrp.Message)
	Update(norn model.Norn)
}

type FailureMessage struct {
	Text         string `json:"text"`
	CutOffPeriod string `json:"cut_off_period"`
}

const (
	defaultMinQueueSize = 5
	minMaxWorkers       = 5
)

// failureText is human readable text for the failure message
const FailureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`
