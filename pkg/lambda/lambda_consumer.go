package typequeue_lambda

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	typequeue "github.com/kvizdos/typequeue/pkg"
	"github.com/sirupsen/logrus"
)

type LambdaConsumer[T typequeue.SQSAbleMessage] struct {
	Logger typequeue.Logger

	SQSEvents events.SQSEvent

	BatchItemFailures []map[string]interface{}
}

func (m *LambdaConsumer[T]) GetBatchItemFailures() map[string]interface{} {
	return map[string]interface{}{
		"batchItemFailures": m.BatchItemFailures,
	}
}

func (m *LambdaConsumer[T]) Ack(receiptID *string) error {
	m.Logger.Debugf("typequeue: acknowledging %s", *receiptID)
	return nil // Not used in Lambda Consumer (is used in Mono)
}

func (m *LambdaConsumer[T]) Reject(messageID *string) error {
	m.Logger.Debugf("typequeue: rejecting %s", *messageID)
	m.BatchItemFailures = append(m.BatchItemFailures, map[string]interface{}{
		"itemIdentifier": *messageID,
	})
	return nil
}

func (m *LambdaConsumer[T]) Consume(ctx context.Context, opts typequeue.ConsumerSQSOptions, processFunc typequeue.TypeQueueProcessingFunc[T]) error {
	// Clear any previous failures.
	m.BatchItemFailures = []map[string]interface{}{}

	// Process each SQS message from the event
	for _, record := range m.SQSEvents.Records {
		var unmarshaled T
		if err := json.Unmarshal([]byte(record.Body), &unmarshaled); err != nil {
			m.Logger.Errorf("typequeue: failed to unmarshal message: %s, error: %v", record.Body, err)
			m.Reject(&record.MessageId)
			continue
		}

		logger := m.Logger
		// Set metadata on the message (e.g., receipt and trace IDs)
		unmarshaled.SetReceiptID(&record.ReceiptHandle)
		if traceAttr, ok := record.MessageAttributes["X-Trace-ID"]; ok && traceAttr.StringValue != nil {
			if _, ok := logger.(*logrus.Logger); ok {
				logger = logger.(*logrus.Logger).WithField("trace-id", *traceAttr.StringValue)
			}
			unmarshaled.SetTraceID(traceAttr.StringValue)
		}

		// Process the message
		if err := processFunc(unmarshaled); err != nil {
			logger.Errorf("typequeue: error processing message: %v", err)
			m.Reject(&record.MessageId)
			continue
		}
	}
	return nil
}
