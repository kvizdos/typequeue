package typequeue_mocks

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kvizdos/typequeue/pkg/typequeue"
)

type MockDispatcher[T typequeue.SQSAbleMessage] struct {
	Messages     map[string][]T
	DispatchChan chan T // Optional channel for live testing
}

func (m *MockDispatcher[T]) Flush() {}

func (m *MockDispatcher[T]) Dispatch(ctx context.Context, event T, targetQueueParameter string, withDelaySeconds ...int64) (*string, error) {
	var traceID string
	if tid, ok := ctx.Value("trace-id").(string); !ok {
		return nil, fmt.Errorf("Missing trace-id")
	} else {
		traceID = tid
	}

	event.SetTraceID(&traceID)

	receiptID := uuid.NewString()
	event.SetReceiptID(&receiptID)

	// If channel is enabled, send to it
	if m.DispatchChan != nil {
		m.DispatchChan <- event
		return &receiptID, nil
	}

	if m.Messages == nil {
		m.Messages = make(map[string][]T) // Initialize the map if it's nil
	}

	// Append the event to the target queue's message list
	m.Messages[targetQueueParameter] = append(m.Messages[targetQueueParameter], event)
	return &receiptID, nil
}
