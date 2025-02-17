package typequeue_lambda_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	tq "github.com/kvizdos/typequeue/pkg"
	typequeue_lambda "github.com/kvizdos/typequeue/pkg/lambda"
)

type TestMessage struct {
	tq.SQSAble
	Content string `json:"content"`
}

// dummyLogger is a simple logger that does nothing.
type dummyLogger struct{}

func (l dummyLogger) Debugf(format string, args ...interface{}) {}
func (l dummyLogger) Errorf(format string, args ...interface{}) {}
func (l dummyLogger) Panicf(format string, args ...interface{}) {}

func TestConsume_UnmarshalError(t *testing.T) {
	// Create a message that contains invalid JSON.
	invalidBody := "not a valid json"
	messageID := "msg-1"
	receiptHandle := "rh-1"
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId:     messageID,
				ReceiptHandle: receiptHandle,
				Body:          invalidBody,
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"X-Trace-ID": {StringValue: awsString("trace-1")},
				},
			},
		},
	}

	consumer := &typequeue_lambda.LambdaConsumer[*TestMessage]{
		Logger:    dummyLogger{},
		SQSEvents: sqsEvent,
	}

	// A process function that always returns nil (should never be called in this case)
	processFunc := func(msg *TestMessage) error {
		return nil
	}

	ctx := context.Background()
	err := consumer.Consume(ctx, tq.ConsumerSQSOptions{}, processFunc)
	if err != nil {
		t.Fatalf("Consume returned unexpected error: %v", err)
	}

	// Since unmarshaling should fail, BatchItemFailures should have one entry.
	if len(consumer.BatchItemFailures) != 1 {
		t.Errorf("expected 1 failure, got %d", len(consumer.BatchItemFailures))
	}

	failure := consumer.BatchItemFailures[0]
	if failure["itemIdentifier"] != messageID {
		t.Errorf("expected failure message id %s, got %v", messageID, failure["itemIdentifier"])
	}
}

func TestConsume_ProcessError(t *testing.T) {
	// Create a valid DummyMessage as JSON.
	msg := TestMessage{Content: "test content"}
	bodyBytes, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal dummy message: %v", err)
	}

	messageID := "msg-2"
	receiptHandle := "rh-2"
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId:     messageID,
				ReceiptHandle: receiptHandle,
				Body:          string(bodyBytes),
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"X-Trace-ID": {StringValue: awsString("trace-2")},
				},
			},
		},
	}

	consumer := &typequeue_lambda.LambdaConsumer[*TestMessage]{
		Logger:    dummyLogger{},
		SQSEvents: sqsEvent,
	}

	// Process function that always returns an error.
	processFunc := func(msg *TestMessage) error {
		return errors.New("process error")
	}

	ctx := context.Background()
	err = consumer.Consume(ctx, tq.ConsumerSQSOptions{}, processFunc)
	if err != nil {
		t.Fatalf("Consume returned unexpected error: %v", err)
	}

	// Since processing fails, BatchItemFailures should have one entry.
	if len(consumer.BatchItemFailures) != 1 {
		t.Errorf("expected 1 failure, got %d", len(consumer.BatchItemFailures))
	}

	failure := consumer.BatchItemFailures[0]
	if failure["itemIdentifier"] != messageID {
		t.Errorf("expected failure message id %s, got %v", messageID, failure["itemIdentifier"])
	}
}

func TestConsume_Success(t *testing.T) {
	// Create a valid DummyMessage as JSON.
	msg := TestMessage{Content: "success content"}
	bodyBytes, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal dummy message: %v", err)
	}

	messageID := "msg-3"
	receiptHandle := "rh-3"
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId:     messageID,
				ReceiptHandle: receiptHandle,
				Body:          string(bodyBytes),
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"X-Trace-ID": {StringValue: awsString("trace-3")},
				},
			},
		},
	}

	consumer := &typequeue_lambda.LambdaConsumer[*TestMessage]{
		Logger:    dummyLogger{},
		SQSEvents: sqsEvent,
	}

	// Process function that succeeds.
	processFunc := func(msg *TestMessage) error {
		// Optionally, verify that metadata was set.
		if *msg.ReceiptID != receiptHandle {
			return errors.New("receipt handle mismatch")
		}
		if *msg.TraceID != "trace-3" {
			return errors.New("trace id mismatch")
		}
		return nil
	}

	ctx := context.Background()
	err = consumer.Consume(ctx, tq.ConsumerSQSOptions{}, processFunc)
	if err != nil {
		t.Fatalf("Consume returned unexpected error: %v", err)
	}

	// On success, BatchItemFailures should be empty.
	if len(consumer.BatchItemFailures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(consumer.BatchItemFailures))
	}
}

// awsString is a helper to return a pointer to a string literal.
func awsString(s string) *string {
	return &s
}
