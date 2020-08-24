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

package eventParser

import (
	"github.com/go-kit/kit/log"
	"go.uber.org/fx"
)

type EventParserIn struct {
	fx.In

	Lifecycle   fx.Lifecycle
	EventSender EventSenderFunc
	Logger      log.Logger
	Measures    Measures
}

func Provide(in EventParserIn, opt ParserConfig) (*EventParser, error) {
	ep, err := NewEventParser(in.EventSender, &in.Logger, opt, in.Measures)
	if err != nil {
		return nil, err
	}

	in.Lifecycle.Append(fx.Hook{
		OnStart: ep.Start(),
		OnStop:  ep.Stop(),
	})

	return ep, nil

}
