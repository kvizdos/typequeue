package typequeue_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kvizdos/typequeue/pkg/typequeue"
	"github.com/stretchr/testify/assert"
)

// TestIntegrationAsyncDispatcherDispatchAndConsume verifies that messages dispatched via AsyncDispatcher
// are delivered to SQS and can be consumed.
func TestIntegrationAsyncDispatcherDispatchAndConsume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	container, endpoint, err := createLocalStack(ctx)
	assert.NoError(t, err, "failed to start LocalStack container")
	defer container.Terminate(ctx)

	// Create an AWS session that talks to LocalStack.
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(endpoint),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
	})
	assert.NoError(t, err, "failed to create AWS session")
	sqsClient := sqs.New(sess)

	// Create an SQS queue.
	createOut, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String("async-queue"),
	})
	assert.NoError(t, err, "failed to create SQS queue")
	queueURL := *createOut.QueueUrl

	// Instantiate AsyncDispatcher.
	asyncDispatcher := &typequeue.AsyncDispatcher[*TestMessage]{
		Dispatcher: typequeue.Dispatcher[*TestMessage]{
			SQSClient:         sqsClient,
			GetTargetQueueURL: func(target string) (string, error) { return queueURL, nil },
		},
		TargetQueue: queueURL,
		// For successful dispatches, no error handling is expected.
		ErrorHandler: func(fm typequeue.FailedMessage[*TestMessage]) {
			t.Logf("Unexpected failure: %v", fm.Error)
		},
	}
	killCtx, killCancel := context.WithCancel(ctx)
	defer killCancel()
	msgChan := asyncDispatcher.Setup(killCtx, 10)
	asyncDispatcher.Start()

	// Dispatch a test message.
	traceID := "async-trace-1"
	msg := &TestMessage{
		Message: "Async Hello, LocalStack!",
		SQSAble: typequeue.SQSAble{TraceID: aws.String(traceID)},
	}
	msgChan <- msg

	// Give AsyncDispatcher some time to dispatch.
	time.Sleep(500 * time.Millisecond)

	// Now consume the message using Consumer.
	testLogger := &TestLogger{Test: t}
	consumer := typequeue.Consumer[*TestMessage]{
		SQSClient:         sqsClient,
		Logger:            testLogger,
		GetTargetQueueURL: func(target string) (string, error) { return queueURL, nil },
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var consumedMsg *TestMessage
	consumerOpts := typequeue.ConsumerSQSOptions{
		TargetQueue:         queueURL,
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     5,
	}

	consumeCtx, consumeCancel := context.WithCancel(ctx)
	defer consumeCancel()
	go func() {
		consumer.Consume(consumeCtx, consumerOpts, func(received *TestMessage) error {
			consumedMsg = received
			wg.Done()
			return nil
		})
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for message consumption")
	}

	assert.NotNil(t, consumedMsg, "expected to consume a message")
	assert.Equal(t, "Async Hello, LocalStack!", consumedMsg.Message)
	assert.Equal(t, traceID, *consumedMsg.TraceID)

	// Shutdown the AsyncDispatcher.
	killCancel()
	asyncDispatcher.Stop()
}

// TestIntegrationAsyncDispatcherReject verifies that when the consumer rejects a message,
// the AsyncDispatcher's error handler logs the failure.
func TestIntegrationAsyncDispatcherReject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	container, endpoint, err := createLocalStack(ctx)
	assert.NoError(t, err, "failed to start LocalStack container")
	defer container.Terminate(ctx)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(endpoint),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
	})
	assert.NoError(t, err, "failed to create AWS session")
	sqsClient := sqs.New(sess)

	// Create an SQS queue.
	createOut, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String("async-reject-queue"),
	})
	assert.NoError(t, err, "failed to create SQS queue")
	queueURL := *createOut.QueueUrl

	logger := &TestLogger{Test: t}
	// Use a dispatcher that simply returns a successful message.
	dispatcher := typequeue.Dispatcher[*TestMessage]{
		SQSClient:         sqsClient,
		GetTargetQueueURL: func(target string) (string, error) { return queueURL, nil },
	}

	// Instantiate AsyncDispatcher with an error handler that logs rejections.
	asyncDispatcher := &typequeue.AsyncDispatcher[*TestMessage]{
		Dispatcher:  dispatcher,
		TargetQueue: queueURL,
		ErrorHandler: func(fm typequeue.FailedMessage[*TestMessage]) {
			logger.Debugf("typequeue: rejecting %s", *fm.Msg.GetTraceID())
			logger.Errorf("typequeue: error processing message: %s", fm.Error.Error())
		},
	}
	killCtx, killCancel := context.WithCancel(ctx)
	defer killCancel()
	msgChan := asyncDispatcher.Setup(killCtx, 10)
	asyncDispatcher.Start()

	// Dispatch a message that the consumer will reject.
	traceID := "reject-trace-1"
	msg := &TestMessage{
		Message: "Reject Message",
		SQSAble: typequeue.SQSAble{TraceID: aws.String(traceID)},
	}
	msgChan <- msg

	// Start a consumer that always returns an error.
	consumer := typequeue.Consumer[*TestMessage]{
		SQSClient:         sqsClient,
		Logger:            logger,
		GetTargetQueueURL: func(target string) (string, error) { return queueURL, nil },
	}

	var wg sync.WaitGroup
	wg.Add(1)
	consumerOpts := typequeue.ConsumerSQSOptions{
		TargetQueue:         queueURL,
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     5,
	}
	consumeCtx, consumeCancel := context.WithCancel(ctx)
	defer consumeCancel()
	go func() {
		consumer.Consume(consumeCtx, consumerOpts, func(received *TestMessage) error {
			wg.Done()
			return fmt.Errorf("consumer rejection")
		})
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for consumer rejection")
	}

	// Give a moment for the error handler to log.
	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, len(logger.DebugLogs), 1, "expected at least 1 debug log for rejection")
	assert.GreaterOrEqual(t, len(logger.ErrorLogs), 1, "expected at least 1 error log for rejection")

	killCancel()
	asyncDispatcher.Stop()
}

// TestIntegrationAsyncDispatcherShutdownDrainsAllMessages verifies that when shutting down,
// the AsyncDispatcher drains all pending messages.
func TestIntegrationAsyncDispatcherShutdownDrainsAllMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	container, endpoint, err := createLocalStack(ctx)
	assert.NoError(t, err, "failed to start LocalStack container")
	defer container.Terminate(ctx)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(endpoint),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
	})
	assert.NoError(t, err, "failed to create AWS session")
	sqsClient := sqs.New(sess)

	// Create an SQS queue.
	createOut, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String("async-shutdown-queue"),
	})
	assert.NoError(t, err, "failed to create SQS queue")
	queueURL := *createOut.QueueUrl

	// Instantiate AsyncDispatcher.
	asyncDispatcher := &typequeue.AsyncDispatcher[*TestMessage]{
		Dispatcher: typequeue.Dispatcher[*TestMessage]{
			SQSClient:         sqsClient,
			GetTargetQueueURL: func(target string) (string, error) { return queueURL, nil },
		},
		TargetQueue:  queueURL,
		ErrorHandler: func(fm typequeue.FailedMessage[*TestMessage]) {},
	}
	killCtx, killCancel := context.WithCancel(ctx)
	msgChan := asyncDispatcher.Setup(killCtx, 20)
	asyncDispatcher.Start()

	// Dispatch several messages.
	totalMessages := 10
	for i := range totalMessages {
		msg := &TestMessage{
			Message: fmt.Sprintf("shutdown message %d", i),
			SQSAble: typequeue.SQSAble{TraceID: aws.String(fmt.Sprintf("trace-%d", i))},
		}
		msgChan <- msg
	}

	// Signal shutdown.
	killCancel()
	asyncDispatcher.Stop()

	consumerLogger := &TestLogger{
		Test: t,
	}
	// Consume all messages to verify they were processed.
	consumer := typequeue.Consumer[*TestMessage]{
		SQSClient:         sqsClient,
		Logger:            consumerLogger,
		GetTargetQueueURL: func(target string) (string, error) { return queueURL, nil },
	}
	var wg sync.WaitGroup
	wg.Add(totalMessages)
	var processedMessages []string
	consumerOpts := typequeue.ConsumerSQSOptions{
		TargetQueue:         queueURL,
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     5,
	}
	consumeCtx, consumeCancel := context.WithCancel(ctx)
	defer consumeCancel()
	go func() {
		consumer.Consume(consumeCtx, consumerOpts, func(received *TestMessage) error {
			processedMessages = append(processedMessages, received.Message)
			wg.Done()
			return nil
		})
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for consumption of all messages")
	}

	assert.Equal(t, totalMessages, len(processedMessages), "all messages should be consumed")
}
