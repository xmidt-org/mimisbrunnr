// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/json"
	"time"

	argus "github.com/xmidt-org/argus/model"
)

type Norn struct {
	DeviceID    string
	ExpiresAt   int64
	Destination Destination
}

type NornRequest struct {
	DeviceID    string
	TTL         int64
	Destination Destination
}

type Destination struct {
	AWSConfig  AWSConfig
	HttpConfig HttpConfig
}

type AWSConfig struct {
	AccessKey string
	SecretKey string
	ID        string
	Sqs       SQSConfig
}

type SQSConfig struct {
	QueueURL     string
	DelaySeconds int64
	Region       string
}

type HttpConfig struct {
	URL        string
	Secret     string
	FailureURL string
}

func NewNorn(nr *NornRequest, ip string) (*Norn, error) {
	return &Norn{
		DeviceID:    nr.DeviceID,
		ExpiresAt:   (time.Now().Add(time.Second * time.Duration(nr.TTL))).Unix(),
		Destination: nr.Destination,
	}, nil
}

func NewNornRequest(jsonString []byte, ip string) (*NornRequest, error) {
	nornReq := new(NornRequest)
	err := json.Unmarshal(jsonString, nornReq)
	if err != nil {
		return &NornRequest{}, err
	}

	return nornReq, nil
}

func ConvertItemToNorn(item argus.Item) (Norn, error) {
	norn := Norn{}
	tempBytes, err := json.Marshal(&item.Data)
	if err != nil {
		return norn, err
	}
	err = json.Unmarshal(tempBytes, &norn)
	if err != nil {
		return norn, err
	}
	return norn, nil
}
