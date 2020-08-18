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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/semaphore"
	"github.com/xmidt-org/wrp-go/v2"
)

type SqsDispatcher struct {
	Wg        sync.WaitGroup
	Measures  *Measures
	Logger    log.Logger
	Norn      model.Norn
	Workers   semaphore.Interface
	SqsClient *sqs.SQS
}

func NewSqsDispatcher(dc DispatcherConfig, norn model.Norn, transport http.RoundTripper) (D, error) {
	if dc.QueueSize < defaultMinQueueSize {
		dc.QueueSize = defaultMinQueueSize
	}

	if dc.NumWorkers < minMaxWorkers {
		dc.NumWorkers = minMaxWorkers
	}

	dispatcher := SqsDispatcher{
		Norn: norn,
	}
	err := validateAWSConfig(norn.Destination.AWSConfig)
	if err != nil {
		return nil, err
	}

	return dispatcher, nil
}

func validateAWSConfig(config model.AWSConfig) error {
	if config.AccessKey == "" {
		return fmt.Errorf("invalid AWS accesskey")
	}

	if config.SecretKey == "" {
		return fmt.Errorf("invalid AWS secretkey")
	}

	if config.ID == "" {
		return fmt.Errorf("invalid AWS id")
	}

	if config.Sqs.QueueURL == "" {
		return fmt.Errorf("invalid SQS queueUrl")
	}

	if config.Sqs.Region == "" {
		return fmt.Errorf("invalid SQS region")
	}

	return nil
}

func (s SqsDispatcher) Start(_ context.Context) error {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(s.Norn.Destination.AWSConfig.Sqs.Region),
		Credentials: credentials.NewStaticCredentials(s.Norn.Destination.AWSConfig.ID, s.Norn.Destination.AWSConfig.AccessKey, s.Norn.Destination.AWSConfig.SecretKey),
	})
	if err != nil {
		return err
	}
	s.SqsClient = sqs.New(sess)

	return nil

}

// called to deliver event
func (s SqsDispatcher) Send(msg *wrp.Message) {
	defer func() {
		s.Wg.Done()
		if r := recover(); nil != r {
			s.Measures.DroppedPanicCounter.Add(1.0)
			s.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "goroutine send() panicked",
				"id", s.Norn.DeviceID, "panic", r)
		}
		s.Workers.Release()
		s.Measures.WorkersCount.Add(-1.0)
	}()

	var (
		url   string
		code  string
		event string
	)

	buffer := bytes.NewBuffer([]byte{})
	encoder := wrp.NewEncoder(buffer, wrp.JSON)
	err := encoder.Encode(msg)
	if err != nil {
		s.Measures.DroppedInvalidConfig.Add(1.0)
		s.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to marshal event.")
		return
	}
	body := buffer.Bytes()

	sqsParams := &sqs.SendMessageInput{
		MessageBody:  aws.String(string(body)),
		QueueUrl:     aws.String(s.Norn.Destination.AWSConfig.Sqs.QueueURL),
		DelaySeconds: aws.Int64(s.Norn.Destination.AWSConfig.Sqs.DelaySeconds),
	}
	_, err = s.SqsClient.SendMessage(sqsParams)
	if err != nil {
		s.Measures.DroppedNetworkErrCounter.Add(1.0)
		s.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to send event to sqs.")
		return
	}
	url = s.Norn.Destination.AWSConfig.Sqs.QueueURL
	s.Measures.DeliveryCounter.With("url", url, "code", code, "event", event).Add(1.0)

}

func (s SqsDispatcher) Update(norn model.Norn) {

	if reflect.DeepEqual(s.Norn.Destination.AWSConfig, norn.Destination.AWSConfig) == false {
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(norn.Destination.AWSConfig.Sqs.Region),
			Credentials: credentials.NewStaticCredentials(norn.Destination.AWSConfig.ID, norn.Destination.AWSConfig.AccessKey, norn.Destination.AWSConfig.SecretKey),
		})
		if err != nil {
			s.Logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to create new aws session.")
			return
		}
		s.SqsClient = sqs.New(sess)
		s.Norn.Destination.AWSConfig = norn.Destination.AWSConfig
	}

}
