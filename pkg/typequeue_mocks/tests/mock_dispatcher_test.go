package typequeue_mocks_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kvizdos/typequeue/pkg/typequeue"
	"github.com/kvizdos/typequeue/pkg/typequeue_mocks"
	"github.com/stretchr/testify/assert"
)

var _ typequeue.TypeQueueDispatcher[*TestMessage] = &typequeue_mocks.MockDispatcher[*TestMessage]{}

// TestDispatchWithoutChannel verifies that Dispatch stores the message in the map when DispatchChan is nil.
func TestDispatchWithoutChannel(t *testing.T) {
	// Create a dispatcher without a channel.
	dispatcher := typequeue_mocks.MockDispatcher[*TestMessage]{
		Messages: nil,
	}

	// Create a context with a "trace-id".
	traceID := uuid.NewString()
	ctx := context.WithValue(context.Background(), "trace-id", traceID)

	// Dispatch a test message.
	testMsg := &TestMessage{Content: "Hello from map"}
	targetQueue := "test-queue"

	_, err := dispatcher.Dispatch(ctx, testMsg, targetQueue)
	assert.NoError(t, err)

	// Verify that the message was added to the map.
	messages, ok := dispatcher.Messages[targetQueue]
	assert.True(t, ok, "expected target queue key to exist in Messages map")
	assert.Equal(t, 1, len(messages), "expected exactly one message in the target queue")
	// Verify that the TraceID was set.
	assert.Equal(t, traceID, *testMsg.TraceID)
}

// TestDispatchWithChannel verifies that Dispatch sends the message over DispatchChan when it is non-nil.
func TestDispatchWithChannel(t *testing.T) {
	// Create a channel for live testing.
	dispatchChan := make(chan *TestMessage, 1)
	dispatcher := typequeue_mocks.MockDispatcher[*TestMessage]{
		DispatchChan: dispatchChan,
		Messages:     make(map[string][]*TestMessage), // even though this branch isn't used
	}

	// Create a context with a "trace-id".
	traceID := uuid.NewString()
	ctx := context.WithValue(context.Background(), "trace-id", traceID)

	// Dispatch a test message.
	testMsg := &TestMessage{Content: "Hello from channel"}
	targetQueue := "test-queue"

	_, err := dispatcher.Dispatch(ctx, testMsg, targetQueue)
	assert.NoError(t, err)

	// Since we're using a channel, the message should be sent to the channel immediately.
	var receivedMsg *TestMessage
	// Use a WaitGroup and timeout to ensure we receive the message.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		receivedMsg = <-dispatchChan
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Check that the message is as expected.
		assert.NotNil(t, receivedMsg)
		assert.Equal(t, "Hello from channel", receivedMsg.Content)
		// The dispatcher branch that sends to the channel should have set the TraceID.
		assert.Equal(t, traceID, *receivedMsg.TraceID)
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for message from DispatchChan")
	}
}

// TestDispatchMissingTraceID verifies that Dispatch returns an error if the context lacks a "trace-id".
func TestDispatchMissingTraceID(t *testing.T) {
	dispatcher := typequeue_mocks.MockDispatcher[*TestMessage]{}
	ctx := context.Background() // No trace-id provided.
	testMsg := &TestMessage{Content: "No trace id"}
	targetQueue := "test-queue"

	_, err := dispatcher.Dispatch(ctx, testMsg, targetQueue)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Missing trace-id")
}
