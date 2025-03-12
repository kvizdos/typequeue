package typequeue_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kvizdos/typequeue/pkg/typequeue"
	"github.com/stretchr/testify/assert"
)

var _ typequeue.TypeQueueBatchDispatcher[*TestMessage] = &typequeue.BatchedDispatcher[*TestMessage]{}

// TestBatchedDispatcherFlush verifies that if a partial batch exists (less than 10 messages),
// canceling the context flushes the remaining messages.
func TestBatchedDispatcherFlush(t *testing.T) {
	var mu sync.Mutex
	deliveredMessages := 0

	fakeClient := &fakeSQSClient{
		SendMessageBatchFn: func(input *sqs.SendMessageBatchInput) (*sqs.SendMessageBatchOutput, error) {
			// Simulate network delay.
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			deliveredMessages += len(input.Entries)
			mu.Unlock()
			return &sqs.SendMessageBatchOutput{}, nil
		},
	}

	logger := &TestLogger{}

	dispatcher := typequeue.NewBatchedDispatcher[*TestMessage](context.Background(), logger, fakeClient, func(target string) (string, error) {
		return target, nil
	}, 100, 5)

	// Dispatch 5 messages (less than the batch size of 10).
	for range 5 {
		// Use a context with a valid trace-id.
		ctxWithTrace := context.WithValue(context.Background(), "trace-id", "trace-flush")
		msg := &TestMessage{Message: "partial batch message"}
		_, err := dispatcher.Dispatch(ctxWithTrace, msg, "fake-queue")
		assert.NoError(t, err)
	}

	// Cancel context to flush the partial batch.
	dispatcher.Flush()

	mu.Lock()
	totalDelivered := deliveredMessages
	mu.Unlock()
	assert.Equal(t, 5, totalDelivered, "Expected 5 messages to be delivered after flush on cancel")
}

// TestBatchedDispatcherFullBatch verifies that dispatching 10 messages immediately triggers
// a batch delivery.
func TestBatchedDispatcherFullBatch(t *testing.T) {
	var mu sync.Mutex
	deliveredMessages := 0
	batchCalls := 0

	fakeClient := &fakeSQSClient{
		SendMessageBatchFn: func(input *sqs.SendMessageBatchInput) (*sqs.SendMessageBatchOutput, error) {
			// Simulate a slight network delay.
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			deliveredMessages += len(input.Entries)
			batchCalls++
			mu.Unlock()
			return &sqs.SendMessageBatchOutput{}, nil
		},
	}

	logger := &TestLogger{}

	dispatcher := typequeue.NewBatchedDispatcher[*TestMessage](context.Background(), logger, fakeClient, func(target string) (string, error) {
		return target, nil
	}, 100, 5)

	ctxWithTrace := context.WithValue(context.Background(), "trace-id", "trace-full")
	// Dispatch 10 messages to trigger a full batch.
	msg := &TestMessage{Message: "full batch message"}
	for range 10 {
		_, err := dispatcher.Dispatch(ctxWithTrace, msg, "fake-queue")
		assert.NoError(t, err)
	}

	dispatcher.Flush()

	mu.Lock()
	totalDelivered := deliveredMessages
	totalBatches := batchCalls
	mu.Unlock()

	assert.Equal(t, 10, totalDelivered, "Expected 10 messages to be delivered for a full batch")
	// When the batch flushes on cancellation, the batchHolder should be empty.
	assert.Equal(t, 1, totalBatches, "Expected exactly 1 batch call for a full batch")
}

// TestBatchedDispatcherConcurrentDispatch stresses the dispatcher by concurrently dispatching
// a large number of messages. It verifies that all messages are eventually batched and delivered,
// regardless of network delays.
func TestBatchedDispatcherConcurrentDispatch(t *testing.T) {
	var mu sync.Mutex
	deliveredMessages := 0
	totalBatches := 0

	fakeClient := &fakeSQSClient{
		SendMessageBatchFn: func(input *sqs.SendMessageBatchInput) (*sqs.SendMessageBatchOutput, error) {
			// Simulate variable network delay between 10ms and 20ms.
			delay := time.Duration(10+rand.Intn(10)) * time.Millisecond
			time.Sleep(delay)
			mu.Lock()
			deliveredMessages += len(input.Entries)
			totalBatches += 1
			mu.Unlock()
			return &sqs.SendMessageBatchOutput{}, nil
		},
	}
	logger := &TestLogger{}

	dispatcher := typequeue.NewBatchedDispatcher[*TestMessage](context.Background(), logger, fakeClient, func(target string) (string, error) {
		delay := time.Duration(10+rand.Intn(10)) * time.Millisecond
		time.Sleep(delay)
		return target, nil
	}, 100, 500)

	totalMessages := 1008

	// Dispatch messages concurrently.
	var wg sync.WaitGroup
	for i := range totalMessages {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each goroutine uses its own context with the same trace-id.
			ctxWithTrace := context.WithValue(context.Background(), "trace-id", "trace-concurrent")
			msg := &TestMessage{Message: "concurrent message"}
			_, err := dispatcher.Dispatch(ctxWithTrace, msg, "fake-queue")
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Cancel to flush any remaining messages.
	dispatcher.Flush()

	mu.Lock()
	totalDelivered := deliveredMessages
	mu.Unlock()

	assert.Equal(t, totalMessages, totalDelivered, "Expected all dispatched messages to be delivered")
	assert.Equal(t, 101, totalBatches, "Expected correct number of batches")
}

func BenchmarkBatchDispatcher(b *testing.B) {

	fakeClient := &fakeSQSClient{
		SendMessageBatchFn: func(input *sqs.SendMessageBatchInput) (*sqs.SendMessageBatchOutput, error) {
			return &sqs.SendMessageBatchOutput{}, nil
		},
	}
	logger := &TestLogger{}

	dispatcher := typequeue.NewBatchedDispatcher[*TestMessage](context.Background(), logger, fakeClient, func(target string) (string, error) {
		return target, nil
	}, 100, 500)

	ctxWithTrace := context.WithValue(context.Background(), "trace-id", "trace-concurrent")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Dispatch a dummy event.
		_, err := dispatcher.Dispatch(ctxWithTrace, &TestMessage{Message: "hello"}, "dummyQueue")
		if err != nil {
			b.Fatal(err)
		}
	}
	dispatcher.Flush()
	b.StopTimer()
	// Flush to ensure all messages are processed.
}
