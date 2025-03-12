package typequeue_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kvizdos/typequeue/pkg/typequeue"
	"github.com/stretchr/testify/assert"
)

var _ typequeue.TypeQueueConsumer[*TestMessage] = &typequeue.Consumer[*TestMessage]{}

// For Consumer tests we will simulate a single ReceiveMessage call.
func TestConsumerConsumeSuccess(t *testing.T) {
	traceID := "test-trace-id"
	// Prepare a valid SQS message.
	testMsg := TestMessage{Message: "Hello, Consumer!"}
	bodyBytes, _ := json.Marshal(testMsg)
	sqsMessage := &sqs.Message{
		MessageId:     aws.String("msg-1"),
		ReceiptHandle: aws.String("receipt-1"),
		Body:          aws.String(string(bodyBytes)),
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"X-Trace-ID": {StringValue: aws.String(traceID)},
		},
	}

	// Fake SQS client that returns one message then no messages.
	callCount := 0
	fakeClient := &fakeSQSClient{
		ReceiveMessageFn: func(input *sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error) {
			if callCount == 0 {
				callCount++
				return &sqs.ReceiveMessageOutput{
					Messages: []*sqs.Message{sqsMessage},
				}, nil
			}
			// Return no messages on subsequent calls.
			time.Sleep(10 * time.Millisecond)
			return &sqs.ReceiveMessageOutput{Messages: []*sqs.Message{}}, nil
		},
		DeleteMessageFn: func(queueURL *string, receiptHandle *string) error {
			// For testing, simply return nil.
			return nil
		},
	}
	logger := &TestLogger{}
	consumer := typequeue.Consumer[*TestMessage]{
		SQSClient: fakeClient,
		Logger:    logger,
		GetTargetQueueURL: func(target string) (string, error) {
			return target, nil
		},
	}

	// Create a channel to signal processing is done.
	done := make(chan struct{})
	processFn := func(received *TestMessage) error {
		assert.Equal(t, testMsg.Message, received.Message)
		// Check that trace id was set.
		assert.Equal(t, traceID, *received.TraceID)
		close(done)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run consumer in a goroutine.
	go func() {
		// We only want one round of receive.
		opts := typequeue.ConsumerSQSOptions{
			TargetQueue:         "fake-queue",
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     1,
		}
		consumer.Consume(ctx, opts, processFn)
	}()

	select {
	case <-done:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for consumer to process message")
	}

	// Expect no errors logged.
	assert.Empty(t, logger.ErrorLogs, "No errors should have been logged")
}

func TestConsumerConsumeProcessError(t *testing.T) {
	traceID := "test-trace-id"
	// Prepare a valid SQS message.
	testMsg := TestMessage{Message: "Hello, Consumer!"}
	bodyBytes, _ := json.Marshal(testMsg)
	sqsMessage := &sqs.Message{
		MessageId:     aws.String("msg-2"),
		ReceiptHandle: aws.String("receipt-2"),
		Body:          aws.String(string(bodyBytes)),
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"X-Trace-ID": {StringValue: aws.String(traceID)},
		},
	}

	// Fake SQS client that returns the message.
	callCount := 0
	fakeClient := &fakeSQSClient{
		ReceiveMessageFn: func(input *sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error) {
			if callCount == 0 {
				callCount++
				return &sqs.ReceiveMessageOutput{
					Messages: []*sqs.Message{sqsMessage},
				}, nil
			}
			time.Sleep(10 * time.Millisecond)
			return &sqs.ReceiveMessageOutput{Messages: []*sqs.Message{}}, nil
		},
		DeleteMessageFn: func(queueURL *string, receiptHandle *string) error {
			// Simulate successful deletion.
			return nil
		},
	}
	logger := &TestLogger{}
	consumer := typequeue.Consumer[*TestMessage]{
		SQSClient: fakeClient,
		Logger:    logger,
		GetTargetQueueURL: func(target string) (string, error) {
			return target, nil
		},
	}

	// The process function returns an error.
	processFn := func(_ *TestMessage) error {
		return errors.New("process failure")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a WaitGroup to wait for processing.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		opts := typequeue.ConsumerSQSOptions{
			TargetQueue:         "fake-queue",
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     1,
		}
		consumer.Consume(ctx, opts, func(received *TestMessage) error {
			defer wg.Done()
			return processFn(received)
		})
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Give a moment for logs to flush.
		time.Sleep(10 * time.Millisecond)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for consumer process error")
	}

	assert.NotNil(t, logger.DebugLogs, "DebugLogs should not be nil")
	assert.NotNil(t, logger.ErrorLogs, "ErrorLogs should not be nil")
	// Check that the debug log contains a reject log and an error log exists.
	assert.Contains(t, *logger.DebugLogs[0], "reject", "Expected debug log to indicate rejection")
	assert.Contains(t, *logger.ErrorLogs[0], "process failure", "Expected error log to contain process error")
}
