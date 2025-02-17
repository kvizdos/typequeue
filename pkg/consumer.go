package typequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type Consumer[T SQSAbleMessage] struct {
	SQSClient SQSAPI

	Logger Logger

	GetTargetQueueURL func(string) (string, error)

	_queueURL string
}

func (m *Consumer[T]) Ack(receiptID *string) error {
	m.Logger.Debugf("typequeue: acknowledging %s", *receiptID)
	return DeleteMessage(m.SQSClient, m._queueURL, receiptID)
}

func (m *Consumer[T]) Reject(messageID *string) error {
	m.Logger.Debugf("typequeue: rejecting %s", *messageID)
	return nil // Not used in Mono Consumer (is used in Lambda)
}

// ConsumeSQSMessages is a generic function to consume SQS messages.
func (m *Consumer[T]) Consume(ctx context.Context, opts ConsumerSQSOptions, processFunc TypeQueueProcessingFunc[T]) error {
	queueURL := opts.TargetQueue
	if m.GetTargetQueueURL != nil {
		v, err := m.GetTargetQueueURL(opts.TargetQueue)

		if err != nil {
			return fmt.Errorf("typequeue: failed to get target queue url: %w", err)
		}

		queueURL = v
	}
	m._queueURL = queueURL

	var wg sync.WaitGroup

	failureChan := make(chan<- *string)
	defer close(failureChan)

	for {
		// Check if the context is done.
		// This prevents EOF err if connection is closed.
		select {
		case <-ctx.Done():
			return nil
		default:
			// continue
		}

		// Receive messages from the queue
		resp, err := m.SQSClient.ReceiveMessage(&sqs.ReceiveMessageInput{
			QueueUrl:              aws.String(queueURL),
			MaxNumberOfMessages:   aws.Int64(opts.MaxNumberOfMessages),
			WaitTimeSeconds:       aws.Int64(opts.WaitTimeSeconds),
			MessageAttributeNames: []*string{aws.String("X-Trace-ID")},
		})
		if err != nil {
			m.Logger.Errorf("typequeue: failed to receive messages: %v", err)
			continue
		}

		if len(resp.Messages) == 0 {
			continue
		}

		// Process messages
		for _, msg := range resp.Messages {
			var unmarshaled T
			if err := json.Unmarshal([]byte(*msg.Body), &unmarshaled); err != nil {
				m.Logger.Errorf("typequeue: failed to unmarshal message: %s, error: %v", *msg.Body, err)
				continue
			}
			unmarshaled.SetReceiptID(msg.ReceiptHandle)
			unmarshaled.SetTraceID(msg.MessageAttributes["X-Trace-ID"].StringValue)
			// Run the process function in a goroutine
			wg.Add(1)
			go func(msg T, msgId *string) {
				defer wg.Done()

				err := processFunc(msg)

				if err != nil {
					m.Logger.Errorf("typequeue: error processing message: %s", err.Error())
					m.Reject(msgId)
					return
				}

				err = m.Ack(msg.GetReceiptID())
				if err != nil {
					m.Logger.Errorf("typequeue: failed to acknowledge message (%s): %s", *msgId, err.Error())
				}
			}(unmarshaled, msg.MessageId)
		}

		wg.Wait()
	}
}
