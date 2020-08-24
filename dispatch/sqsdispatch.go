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
	"net/url"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/v2"
)

type SQSDispatcher struct {
	wg        sync.WaitGroup
	measures  Measures
	logger    log.Logger
	awsConfig model.AWSConfig
	sqsClient *sqs.SQS
	mutex     sync.RWMutex
}

func NewSqsDispatcher(dc SenderConfig, awsConfig model.AWSConfig, logger log.Logger, measures Measures) (*SQSDispatcher, error) {

	dispatcher := SQSDispatcher{
		awsConfig: awsConfig,
		logger:    logger,
		measures:  measures,
	}
	err := validateAWSConfig(awsConfig)
	if err != nil {
		return nil, err
	}

	return &dispatcher, nil
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

	_, err := url.ParseRequestURI(config.Sqs.QueueURL)
	if err != nil {
		return fmt.Errorf("invalid SQS queueUrl")
	}

	if config.Sqs.Region == "" {
		return fmt.Errorf("invalid SQS region")
	}

	return nil
}

func (s *SQSDispatcher) Start(_ context.Context) error {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(s.awsConfig.Sqs.Region),
		Credentials: credentials.NewStaticCredentials(s.awsConfig.ID, s.awsConfig.AccessKey, s.awsConfig.SecretKey),
	})
	if err != nil {
		return err
	}
	s.sqsClient = sqs.New(sess)

	return nil

}

// called to deliver event
func (s *SQSDispatcher) Send(msg *wrp.Message) {
	url := s.awsConfig.Sqs.QueueURL
	defer func() {
		s.wg.Done()
		if r := recover(); nil != r {
			s.measures.DroppedPanicCounter.Add(1.0)
			s.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "goroutine send() panicked",
				"url", url, "panic", r)
		}
		s.measures.WorkersCount.Add(-1.0)
	}()

	var (
		code  string
		event string
	)

	buffer := bytes.NewBuffer([]byte{})
	encoder := wrp.NewEncoder(buffer, wrp.JSON)
	err := encoder.Encode(msg)
	if err != nil {
		s.measures.DroppedInvalidConfig.Add(1.0)
		s.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to marshal event.")
		return
	}
	body := buffer.Bytes()
	event = msg.FindEventStringSubMatch()

	sqsParams := &sqs.SendMessageInput{
		MessageBody:  aws.String(string(body)),
		QueueUrl:     aws.String(url),
		DelaySeconds: aws.Int64(s.awsConfig.Sqs.DelaySeconds),
	}
	_, err = s.sqsClient.SendMessage(sqsParams)
	if err != nil {
		s.measures.DroppedNetworkErrCounter.Add(1.0)
		s.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to send event to sqs.")
		return
	}
	s.measures.DeliveryCounter.With("url", url, "code", code, "event", event).Add(1.0)

}

func (s *SQSDispatcher) Update(norn model.Norn) {

	s.mutex.Lock()
	s.awsConfig = norn.Destination.AWSConfig
	s.mutex.Unlock()

	err := s.Start(nil)
	if err != nil {
		fmt.Errorf("failed to update aws session")
		return
	}

}
