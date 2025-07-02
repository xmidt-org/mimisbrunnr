// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package eventParser

import (
	"github.com/go-kit/log"
	"go.uber.org/fx"
)

// ParserIn sets all the dependencies for this package's components
type ParserIn struct {
	fx.In

	Lifecycle   fx.Lifecycle
	EventSender EventSenderFunc
	Logger      log.Logger
	Measures    Measures
}

// Provide is an uber/fx style provider for this package's components
func Provide(in ParserIn, opt ParserConfig) (*EventParser, error) {
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
