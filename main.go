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

package main

import (
	"fmt"
	"mimisbrunnr/eventregistration"
	"mimisbrunnr/webhook"
	"os"

	"github.com/spf13/pflag"
	"go.uber.org/fx"
)

const (
	applicationName = "mimisbrunnr"
)

func main() {
	app := fx.New(
		fx.Provide(
			webhook.Mimisbrunnr,
			eventregistration.Listener,
		),
	)
	fmt.Println(app)
	switch err := app.Err(); err {
	case pflag.ErrHelp:
		return
	case nil:
		app.Run()
	default:
		fmt.Println(os.Stderr, err)
		os.Exit(2)
	}

}
