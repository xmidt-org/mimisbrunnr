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
