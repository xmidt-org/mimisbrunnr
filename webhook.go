// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"time"

	"github.com/xmidt-org/bascule/acquire"
	webhook "github.com/xmidt-org/wrp-listener"
	"github.com/xmidt-org/wrp-listener/webhookClient"
)

type WebhookConfig struct {
	RegistrationInterval time.Duration
	Timeout              time.Duration
	RegistrationURL      string
	HostToRegister       string
	Request              webhook.W
	JWT                  acquire.RemoteBearerTokenAcquirerOptions
	Basic                string
}

// determineTokenAcquirer always returns a valid TokenAcquirer
func determineTokenAcquirer(config WebhookConfig) (webhookClient.Acquirer, error) {
	defaultAcquirer := &acquire.DefaultAcquirer{}
	if config.JWT.AuthURL != "" && config.JWT.Buffer != 0 && config.JWT.Timeout != 0 {
		return acquire.NewRemoteBearerTokenAcquirer(config.JWT)
	}

	if config.Basic != "" {
		return acquire.NewFixedAuthAcquirer(config.Basic)
	}

	return defaultAcquirer, nil
}
