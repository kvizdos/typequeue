package typequeue_mocks

import (
	"context"

	"github.com/google/uuid"
	typequeue "github.com/kvizdos/typequeue/pkg"
)

type MockConsumer[T typequeue.SQSAbleMessage] struct {
	QueuedMessages map[*string]T
	MessagesChan   chan T // Optional channel for live testing
	Logger         typequeue.Logger
}

func (m *MockConsumer[T]) Ack(receiptID *string) error {
	delete(m.QueuedMessages, receiptID)
	m.Logger.Debugf("typequeue: acknowledging %s", &receiptID)
	return nil
}
func (m *MockConsumer[T]) Reject(msgID *string) error {
	m.Logger.Debugf("typequeue: rejecting %s", &msgID)
	return nil
}

func (m *MockConsumer[T]) Consume(ctx context.Context, opts typequeue.ConsumerSQSOptions, processFunc typequeue.TypeQueueProcessingFunc[T]) error {
	// If channel is enabled, consume from it
	if m.MessagesChan != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case msg := <-m.MessagesChan:
					receiptID := uuid.NewString()
					msg.SetReceiptID(&receiptID)
					err := processFunc(msg)
					if err != nil {
						m.Logger.Errorf("typequeue: error processing message: %s", err.Error())
					} else {
						m.Logger.Debugf("typequeue: acknowledging: %s", receiptID)
					}
				}
			}
		}()
		return nil
	}

	for receiptID, msg := range m.QueuedMessages {
		trace := uuid.NewString()
		msg.SetReceiptID(receiptID)
		msg.SetTraceID(&trace)
		err := processFunc(msg)
		if err != nil {
			m.Reject(receiptID)
		} else {
			m.Ack(receiptID)
		}
	}

	return nil
}
