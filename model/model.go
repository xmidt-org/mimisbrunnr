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

import "encoding/json"

type Norn struct {
	DeviceID    string
	TTL         int64
	Destination Destination
}

type Destination struct {
	Type       string
	Info       map[string]interface{}
	AWSConfig  AWSConfig
	HttpConfig HttpConfig
}

type AWSConfig struct {
	AccessKey string
	SecretKey string
	ID        string
	Env       string
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
	AcceptType string
}

func NewNorn(jsonString []byte, ip string) (norn *Norn, err error) {
	norn = new(Norn)

	err = json.Unmarshal(jsonString, norn)
	if err != nil {
		var norns []Norn

		err = json.Unmarshal(jsonString, &norns)
		if err != nil {
			return
		}
		norn = &norns[0]
	}

	err = norn.sanitize(ip)
	if nil != err {
		norn = nil
	}
	return
}

// add santize stuff here
func (norn *Norn) sanitize(ip string) (err error) {
	return
}
