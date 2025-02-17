package typequeue_test

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	typequeue "github.com/kvizdos/typequeue/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMessage is a simple implementation of the SQSAbleMessage interface for testing.
type TestMessage struct {
	typequeue.SQSAble
	Message string `json:"msg"`
}

type TestLogger struct {
	log.Logger
	Test *testing.T

	DebugLogs []*string
	ErrorLogs []*string
	mutex     sync.Mutex
}

func (t *TestLogger) Panicf(msg string, args ...any) {
	t.Test.Errorf("PANIC: %s", fmt.Sprintf(msg, args...))
	return
}
func (l *TestLogger) Errorf(format string, v ...any) {
	l.mutex.Lock()
	if l.DebugLogs == nil {
		l.ErrorLogs = []*string{}
	}
	o := fmt.Sprintf(format, v...)
	l.ErrorLogs = append(l.DebugLogs, &o)
	l.mutex.Unlock()
	log.Printf("Error -- %s", fmt.Sprintf(format, v...))
	return
}

func (l *TestLogger) Debugf(format string, v ...interface{}) {
	l.mutex.Lock()
	if l.DebugLogs == nil {
		l.DebugLogs = []*string{}
	}
	o := fmt.Sprintf(format, v...)
	l.DebugLogs = append(l.DebugLogs, &o)
	l.mutex.Unlock()
	log.Printf("Debug -- %s", fmt.Sprintf(format, v...))
}

// createLocalStack starts a LocalStack container with SQS enabled and returns its endpoint.
func createLocalStack(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "localstack/localstack:latest",
		ExposedPorts: []string{"4566/tcp"},
		Env: map[string]string{
			"SERVICES": "sqs",
		},
		WaitingFor: wait.ForLog("Ready."), // Wait until LocalStack is ready
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	mappedPort, err := container.MappedPort(ctx, "4566")
	if err != nil {
		return nil, "", err
	}
	endpoint := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	return container, endpoint, nil
}

func TestIntegrationDispatchAndConsume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	container, endpoint, err := createLocalStack(ctx)
	if err != nil {
		t.Fatalf("failed to start LocalStack container: %v", err)
	}
	defer container.Terminate(ctx)

	// Create an AWS session that talks to LocalStack.
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(endpoint),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
	})
	if err != nil {
		t.Fatalf("failed to create AWS session: %v", err)
	}
	sqsClient := sqs.New(sess)

	// Create an SQS queue.
	createOut, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String("test-queue"),
	})
	if err != nil {
		t.Fatalf("failed to create SQS queue: %v", err)
	}
	queueURL := *createOut.QueueUrl

	// Instantiate the Dispatcher.
	dispatcher := typequeue.Dispatcher[*TestMessage]{
		SQSClient: sqsClient,
		// In this test, our "targetQueue" is already the URL.
		GetTargetQueueURL: func(target string) (string, error) {
			return queueURL, nil
		},
	}

	logger := &TestLogger{
		Test: t,
	}
	// Instantiate the Consumer.
	consumer := typequeue.Consumer[*TestMessage]{
		SQSClient: sqsClient,
		Logger:    logger,
		GetTargetQueueURL: func(target string) (string, error) {
			return queueURL, nil
		},
	}

	// Dispatch an event.
	traceID := "test-trace-id"
	event := &TestMessage{Message: "Hello, LocalStack!"}
	dispatchCtx := context.WithValue(ctx, "trace-id", traceID)
	_, err = dispatcher.Dispatch(dispatchCtx, event, queueURL)
	assert.NoError(t, err, "No error expected on Dispatch")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a WaitGroup to wait until the consumer processes the message.
	var wg sync.WaitGroup
	wg.Add(1)

	opts := typequeue.ConsumerSQSOptions{
		TargetQueue:         queueURL,
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     5,
	}

	// Start the consumer in a separate goroutine.
	go func() {
		consumer.Consume(ctx, opts, func(received *TestMessage) error {
			if received.Message != "Hello, LocalStack!" {
				t.Errorf("expected message 'Hello, LocalStack!', got '%s'", received.Message)
			}
			if *received.TraceID != traceID {
				t.Errorf("expected trace id '%s', got '%s'", traceID, *received.TraceID)
			}
			wg.Done()
			return nil
		})
	}()

	// Wait for the consumer to process the message (with a timeout).
	done := make(chan struct{})
	go func() {
		wg.Wait()
		ctx.Done()
		close(done)
	}()

	select {
	case <-done:
		time.Sleep(10 * time.Millisecond) // race condition fix
		assert.Nil(t, logger.ErrorLogs, "expected nothing to be errored")
		// Success.
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for message consumption")
	}
}

func TestDispatchAndConsumeReject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	container, endpoint, err := createLocalStack(ctx)
	if err != nil {
		t.Fatalf("failed to start LocalStack container: %v", err)
	}
	defer container.Terminate(ctx)

	// Create an AWS session that talks to LocalStack.
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(endpoint),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
	})
	if err != nil {
		t.Fatalf("failed to create AWS session: %v", err)
	}
	sqsClient := sqs.New(sess)

	// Create an SQS queue.
	createOut, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String("test-queue"),
	})
	if err != nil {
		t.Fatalf("failed to create SQS queue: %v", err)
	}
	queueURL := *createOut.QueueUrl

	// Instantiate the Dispatcher.
	dispatcher := typequeue.Dispatcher[*TestMessage]{
		SQSClient: sqsClient,
		// In this test, our "targetQueue" is already the URL.
		GetTargetQueueURL: func(target string) (string, error) {
			return queueURL, nil
		},
	}

	logger := &TestLogger{
		Test: t,
	}
	// Instantiate the Consumer.
	consumer := typequeue.Consumer[*TestMessage]{
		SQSClient: sqsClient,
		Logger:    logger,
		GetTargetQueueURL: func(target string) (string, error) {
			return queueURL, nil
		},
	}

	// Dispatch an event.
	traceID := "test-trace-id"
	event := &TestMessage{Message: "Hello, LocalStack!"}
	dispatchCtx := context.WithValue(ctx, "trace-id", traceID)
	msgId, err := dispatcher.Dispatch(dispatchCtx, event, queueURL)
	assert.NoError(t, err, "No error expected on Dispatch")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a WaitGroup to wait until the consumer processes the message.
	var wg sync.WaitGroup
	wg.Add(1)

	opts := typequeue.ConsumerSQSOptions{
		TargetQueue:         queueURL,
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     5,
	}

	// Start the consumer in a separate goroutine.
	go func() {
		consumer.Consume(ctx, opts, func(received *TestMessage) error {
			defer wg.Done()
			return fmt.Errorf("test error")
		})
	}()

	// Wait for the consumer to process the message (with a timeout).
	done := make(chan struct{})
	go func() {
		wg.Wait()
		ctx.Done()
		close(done)
	}()

	select {
	case <-done:
		time.Sleep(10 * time.Millisecond) // race condition fix
		assert.Len(t, logger.DebugLogs, 1, "Expected 1 log in Debug")
		assert.Equal(t, fmt.Sprintf("typequeue: rejecting %s", *msgId), *logger.DebugLogs[0])
		assert.Len(t, logger.ErrorLogs, 1, "Expected 1 log in Error")
		assert.Equal(t, "typequeue: error processing message: test error", *logger.ErrorLogs[0])
		// Success.
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for message consumption")
	}

}
