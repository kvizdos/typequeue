package typequeue_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kvizdos/typequeue/pkg/typequeue"
	"github.com/stretchr/testify/assert"
)

// fakeDispatcher implements TypeQueueDispatcher[*TestMessage].
type fakeDispatcher struct {
	DispatchFn func(ctx context.Context, event *TestMessage, targetQueue string, withDelaySeconds ...int64) (*string, error)
}

func (f *fakeDispatcher) Dispatch(ctx context.Context, event *TestMessage, targetQueue string, withDelaySeconds ...int64) (*string, error) {
	return f.DispatchFn(ctx, event, targetQueue, withDelaySeconds...)
}

// ptr is a helper to return a pointer to a string.
func ptr(s string) *string {
	return &s
}

func TestAsyncDispatcher_Success(t *testing.T) {
	// Create a cancellable context to simulate shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a channel to signal that the fake dispatch was called.
	dispatched := make(chan bool, 1)
	fakeDisp := &fakeDispatcher{
		DispatchFn: func(ctx context.Context, event *TestMessage, targetQueue string, withDelaySeconds ...int64) (*string, error) {
			// Signal that Dispatch was called and simulate success.
			dispatched <- true
			return ptr("msg-success"), nil
		},
	}

	// Set up an error handler that will mark if it's been called.
	var mu sync.Mutex
	errorHandlerCalled := false
	errorHandler := func(fm typequeue.FailedMessage[*TestMessage]) {
		mu.Lock()
		errorHandlerCalled = true
		mu.Unlock()
	}

	// Create and set up the AsyncDispatcher.
	bd := &typequeue.AsyncDispatcher[*TestMessage]{
		Dispatcher:   fakeDisp,
		TargetQueue:  "test-queue",
		ErrorHandler: errorHandler,
	}
	msgChan := bd.Setup(ctx, 10)
	bd.Start()

	// Send a test message that should dispatch successfully.
	testMsg := &TestMessage{
		Message: "hello background",
		SQSAble: typequeue.SQSAble{
			TraceID: ptr("trace-success"),
		},
	}
	msgChan <- testMsg

	// Wait until Dispatch is called.
	select {
	case <-dispatched:
		// ok
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Dispatcher did not process the message in time")
	}

	// Signal shutdown.
	cancel()
	bd.Stop()

	mu.Lock()
	handlerCalled := errorHandlerCalled
	mu.Unlock()
	assert.False(t, handlerCalled, "Error handler should not have been called for successful dispatch")
}

func TestAsyncDispatcher_Failure(t *testing.T) {
	// Create a cancellable context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a channel to signal that Dispatch was called.
	dispatched := make(chan bool, 1)
	fakeDisp := &fakeDispatcher{
		DispatchFn: func(ctx context.Context, event *TestMessage, targetQueue string, withDelaySeconds ...int64) (*string, error) {
			dispatched <- true
			// Simulate failure.
			return nil, fmt.Errorf("dispatch failure")
		},
	}

	// Use an error handler that collects failed messages.
	var mu sync.Mutex
	var failedMessages []typequeue.FailedMessage[*TestMessage]
	errorHandler := func(fm typequeue.FailedMessage[*TestMessage]) {
		mu.Lock()
		failedMessages = append(failedMessages, fm)
		mu.Unlock()
	}

	bd := &typequeue.AsyncDispatcher[*TestMessage]{
		Dispatcher:   fakeDisp,
		TargetQueue:  "test-queue",
		ErrorHandler: errorHandler,
	}
	msgChan := bd.Setup(ctx, 10)
	bd.Start()

	// Send a test message that will fail.
	testMsg := &TestMessage{
		Message: "fail background",
		SQSAble: typequeue.SQSAble{
			TraceID: ptr("trace-failure"),
		},
	}
	msgChan <- testMsg

	// Wait until Dispatch is called.
	select {
	case <-dispatched:
		// ok
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Dispatcher did not process the message in time")
	}

	// Signal shutdown.
	cancel()
	bd.Stop()

	mu.Lock()
	numFailures := len(failedMessages)
	mu.Unlock()
	assert.Equal(t, 1, numFailures, "Expected one failed message")
	if numFailures > 0 {
		assert.Equal(t, "dispatch failure", failedMessages[0].Error.Error())
		assert.Equal(t, testMsg, failedMessages[0].Msg)
	}
}

// TestAsyncDispatcher_ShutdownDrainsAllMessages verifies that shutdown drains all messages
// (successful dispatches in this case) from the msgChan.
func TestAsyncDispatcher_ShutdownDrainsAllMessages(t *testing.T) {
	// Create a context that we won't cancel immediately.
	ctx, cancel := context.WithCancel(context.Background())
	// We'll cancel later to trigger shutdown.
	defer cancel()

	var mu sync.Mutex
	processedMessages := make([]*TestMessage, 0)
	// Use a fake dispatcher that simulates a slight delay.
	fakeDisp := &fakeDispatcher{
		DispatchFn: func(ctx context.Context, event *TestMessage, targetQueue string, withDelaySeconds ...int64) (*string, error) {
			// Simulate some processing delay.
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			processedMessages = append(processedMessages, event)
			mu.Unlock()
			return ptr("processed"), nil
		},
	}

	errorHandler := func(fm typequeue.FailedMessage[*TestMessage]) {
		// In this test, we do not expect failures.
	}

	asyncDispatcher := &typequeue.AsyncDispatcher[*TestMessage]{
		Dispatcher:   fakeDisp,
		TargetQueue:  "dummy-queue",
		ErrorHandler: errorHandler,
	}
	msgChan := asyncDispatcher.Setup(ctx, 20)
	asyncDispatcher.Start()

	totalMessages := 10
	for i := range totalMessages {
		msg := &TestMessage{
			Message: fmt.Sprintf("shutdown message %d", i),
			SQSAble: typequeue.SQSAble{
				TraceID: ptr(fmt.Sprintf("trace-%d", i)),
			},
		}
		msgChan <- msg
	}

	// Now cancel the context to signal shutdown.
	cancel()
	asyncDispatcher.Stop()

	// After Stop() returns, all messages should have been drained and processed.
	mu.Lock()
	processedCount := len(processedMessages)
	mu.Unlock()

	assert.Equal(t, totalMessages, processedCount, "All messages should be processed during shutdown")
}
