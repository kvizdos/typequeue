package typequeue_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	typequeue "github.com/kvizdos/typequeue/pkg"
	"github.com/stretchr/testify/assert"
)

func TestDispatcherDispatchMissingTraceID(t *testing.T) {
	// Create a fake SQS client that never gets called.
	fakeClient := &fakeSQSClient{
		SendMessageFn: func(input *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
			return nil, nil
		},
	}
	dispatcher := typequeue.Dispatcher[*TestMessage]{
		SQSClient: fakeClient,
		// We return the provided target without any change.
		GetTargetQueueURL: func(target string) (string, error) {
			return target, nil
		},
	}

	// Create a context without a trace id.
	ctx := context.Background()
	event := &TestMessage{Message: "Hello"}
	_, err := dispatcher.Dispatch(ctx, event, "fake-queue")
	assert.Error(t, err, "Expected error when trace-id is missing")
}

func TestDispatcherDispatchSuccess(t *testing.T) {
	traceID := "test-trace-id"
	fakeMessageID := "fake-msg-id"
	var capturedInput *sqs.SendMessageInput

	// Fake SQS client that captures the input and returns a message ID.
	fakeClient := &fakeSQSClient{
		SendMessageFn: func(input *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
			capturedInput = input
			return &sqs.SendMessageOutput{
				MessageId: aws.String(fakeMessageID),
			}, nil
		},
	}
	dispatcher := typequeue.Dispatcher[*TestMessage]{
		SQSClient: fakeClient,
		GetTargetQueueURL: func(target string) (string, error) {
			return target, nil
		},
	}
	ctx := context.WithValue(context.Background(), "trace-id", traceID)
	event := &TestMessage{Message: "Hello, Unit!"}

	// Test with a delay provided.
	delay := int64(5)
	msgID, err := dispatcher.Dispatch(ctx, event, "fake-queue", delay)
	assert.NoError(t, err, "Dispatch should succeed")
	assert.Equal(t, fakeMessageID, *msgID)
	assert.NotNil(t, capturedInput)

	// Verify the message body is correct.
	var unmarshaled TestMessage
	err = json.Unmarshal([]byte(*capturedInput.MessageBody), &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, event.Message, unmarshaled.Message)

	// Verify delay and message attribute.
	assert.Equal(t, delay, *capturedInput.DelaySeconds)
	assert.Equal(t, traceID, *capturedInput.MessageAttributes["X-Trace-ID"].StringValue)
}
