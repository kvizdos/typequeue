package typequeue_mocks_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	typequeue "github.com/kvizdos/typequeue/pkg"
	typequeue_mocks "github.com/kvizdos/typequeue/pkg/mocked"
	"github.com/stretchr/testify/assert"
)

// TestMessage is a simple implementation of the SQSAbleMessage interface for testing.
type TestMessage struct {
	typequeue.SQSAble
	Content string `json:"content"`
}

var _ typequeue.TypeQueueConsumer[*TestMessage] = &typequeue_mocks.MockConsumer[*TestMessage]{}

// TestConsumeWithoutChannel verifies that Consume correctly processes messages
// from the QueuedMessages map when MessagesChan is nil.
func TestConsumeWithoutChannel(t *testing.T) {
	// Prepare a test receipt ID and a test message.
	receipt := uuid.NewString()
	msg := &TestMessage{
		Content: "Hello from map",
	}
	// Create a map with one entry. Note: we use a pointer to receipt as the key.
	queuedMessages := make(map[*string]*TestMessage)
	queuedMessages[&receipt] = msg

	// Create the consumer with the queued messages and no channel.
	consumer := typequeue_mocks.MockConsumer[*TestMessage]{
		QueuedMessages: queuedMessages,
		Logger: &TestLogger{
			Test: t,
		},
	}

	// Use a WaitGroup to wait for the processFunc to complete.
	var wg sync.WaitGroup
	wg.Add(1)

	// processFunc should receive one message and call the ack function.
	processFunc := func(receivedMsg *TestMessage) error {
		defer wg.Done()
		assert.NotEmpty(t, receivedMsg.ReceiptID)
		assert.NotEmpty(t, receivedMsg.TraceID)

		assert.Equal(t, "Hello from map", receivedMsg.Content)
		return nil
	}

	// Call Consume. In this branch, Consume will iterate the map and call processFunc synchronously.
	err := consumer.Consume(context.Background(), typequeue.ConsumerSQSOptions{}, processFunc)
	assert.NoError(t, err)

	// Wait for the processFunc to finish.
	wg.Wait()

	// After ack, the message should have been deleted from the map.
	_, exists := consumer.QueuedMessages[&receipt]
	assert.False(t, exists, "Message should be acknowledged and removed")
}

// TestConsumeWithChannel verifies that Consume correctly processes messages
// when the MessagesChan is enabled.
func TestConsumeWithChannel(t *testing.T) {
	// Create a consumer with a buffered channel.
	msgChan := make(chan *TestMessage, 1)
	consumer := typequeue_mocks.MockConsumer[*TestMessage]{
		QueuedMessages: make(map[*string]*TestMessage),
		MessagesChan:   msgChan,
		Logger: &TestLogger{
			Test: t,
		},
	}

	// Use a WaitGroup to wait for the processFunc to complete.
	var wg sync.WaitGroup
	wg.Add(1)

	// processFunc should be called when a message is received from the channel.
	processFunc := func(receivedMsg *TestMessage) error {
		defer wg.Done()
		// The ReceiptID should have been set in the channel branch.
		assert.NotEmpty(t, receivedMsg.ReceiptID)

		assert.Equal(t, "Hello from channel", receivedMsg.Content)
		return nil
	}

	// Start consumption. The goroutine will run indefinitely, so we only care about one message.
	err := consumer.Consume(context.Background(), typequeue.ConsumerSQSOptions{}, processFunc)
	assert.NoError(t, err)

	// Create a test message and send it on the channel.
	testMsg := &TestMessage{Content: "Hello from channel"}
	msgChan <- testMsg

	// Wait for the processFunc to complete.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// Test completed successfully.
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for message processing from channel")
	}
}
