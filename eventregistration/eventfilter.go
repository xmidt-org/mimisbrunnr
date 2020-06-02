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

package eventregistration

import (
	"path"
	"strings"

	db "github.com/xmidt-org/codex-db"
	"github.com/xmidt-org/svalinn/rules"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/v2"
)

func (app *App) EventFilter(message wrp.Message) error {
	var (
		err      error
		deviceID string
	)

	rules, err := rules.NewRules(app.RegexRules)
	if err != nil {
		return err
	}

	rule, err := rules.FindRule(message.Destination)
	if err != nil {
		logging.Info(app.logger).Log(logging.MessageKey(), "Could not get rule", logging.ErrorKey(), err, "destination", message.Destination)
	}

	eventType := db.Default
	if rule != nil {
		eventType = db.ParseEventType(rule.EventType())
	}

	if eventType == db.State {
		// get state and id from dest if this is a state event
		base, _ := path.Split(message.Destination)
		base, deviceId := path.Split(path.Base(base))
		if deviceId == "" {
			// return emptyRecord, parseFailReason, emperror.WrapWith(errEmptyID, "id check failed", "request destination", req.Destination, "full message", req)
		}
		deviceID = strings.ToLower(deviceId)
	} else {
		if message.Source == "" {
			// return emptyRecord, parseFailReason, emperror.WrapWith(errEmptyID, "id check failed", "request Source", req.Source, "full message", req)
		}
		deviceID = strings.ToLower(message.Source)
	}

	if deviceID == app.deviceID {
		//todo: implement to deliver event
	}

	return err

}
