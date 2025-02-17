package typequeue_mocks_test

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	typequeue "github.com/kvizdos/typequeue/pkg"
	typequeue_mocks "github.com/kvizdos/typequeue/pkg/mocked"
	"github.com/stretchr/testify/assert"
)

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

// TestCrossTalk demonstrates live message passing between the
// mock dispatcher and consumer using a shared channel.
func TestCrossTalk(t *testing.T) {
	// Create a shared channel to simulate live message passing.
	sharedChan := make(chan *TestMessage, 10)

	// Initialize the dispatcher and consumer with the shared channel.
	dispatcher := typequeue_mocks.MockDispatcher[*TestMessage]{
		DispatchChan: sharedChan,
		Messages:     make(map[string][]*TestMessage),
	}
	consumer := typequeue_mocks.MockConsumer[*TestMessage]{
		MessagesChan:   sharedChan,
		QueuedMessages: make(map[*string]*TestMessage),
		Logger: &TestLogger{
			Test: t,
		},
	}

	// Create a context with a "trace-id" to simulate production behavior.
	traceID := uuid.NewString()
	ctx := context.WithValue(context.Background(), "trace-id", traceID)

	// Create a test message.
	msg := &TestMessage{
		Content: "Live mock message",
	}

	// Use a WaitGroup to wait for the consumer process function to finish.
	var wg sync.WaitGroup
	wg.Add(1)

	// Define the consumer process function.
	processFunc := func(receivedMsg *TestMessage) error {
		defer wg.Done()
		// Verify the content and trace ID.
		assert.Equal(t, "Live mock message", receivedMsg.Content)
		assert.Equal(t, traceID, *receivedMsg.TraceID)
		return nil
	}

	// Start the consumer in a separate goroutine.
	go func() {
		err := consumer.Consume(context.Background(), typequeue.ConsumerSQSOptions{}, processFunc)
		assert.NoError(t, err)
	}()

	// Dispatch the message.
	_, err := dispatcher.Dispatch(ctx, msg, "dummyQueue")
	assert.NoError(t, err)

	// Wait for the consumer to process the message, with a timeout.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success!
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for message processing in live mock test")
	}
}
