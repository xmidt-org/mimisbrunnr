// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/xmidt-org/mimisbrunnr/model"
	"github.com/xmidt-org/webpa-common/v2/logging" //nolint: staticcheck
	"github.com/xmidt-org/wrp-go/v3"
)

// SQSDispatcher implements the dispatcher interface to send events to sqs.
type SQSDispatcher struct {
	wg        sync.WaitGroup
	measures  Measures
	logger    log.Logger
	awsConfig model.AWSConfig
	sqsClient *sqs.SQS
	mutex     sync.RWMutex
}

// NewSqsDispatcher validates aws configs and creates a sqs dispatcher.
func NewSqsDispatcher(ds *DispatcherSender, awsConfig model.AWSConfig, logger log.Logger, measures Measures) (*SQSDispatcher, error) {
	err := validateAWSConfig(awsConfig)
	if err != nil {
		return nil, err
	}

	dispatcher := SQSDispatcher{
		awsConfig: awsConfig,
		logger:    logger,
		measures:  measures,
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

// Start creates a new aws session with the provided aws configs for event delivery to sqs.
func (s *SQSDispatcher) Start(context.Context) error {
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

// Send uses the configured sqs client to send a WRP message
// as a JSON.
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
		if awsErr, ok := err.(awserr.Error); ok { //nolint: errorlint
			code = awsErr.Code()
		}
		s.measures.DroppedNetworkErrCounter.Add(1.0)
		s.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to send event to sqs.", "url", url, "code", code, "event", event)
		return
	} else {
		code = "200"
	}
	s.measures.DeliveryCounter.With("url", url, "code", code, "event", event).Add(1.0)

}

// Update creates a new aws session with updated credentials for a norn.
func (s *SQSDispatcher) Update(norn model.Norn) {

	s.mutex.Lock()
	s.awsConfig = norn.Destination.AWSConfig
	s.mutex.Unlock()

	err := s.Start(context.TODO())
	if err != nil {
		s.logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Failed to send event to update aws session.", "error", err)
		return
	}

}
