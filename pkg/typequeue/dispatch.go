package typequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
)

func DurationToSeconds(d time.Duration) int64 {
	return int64(d.Seconds())
}

type Dispatcher[T SQSAbleMessage] struct {
	SQSClient SQSAPI
	// Can be used to easily incorporate
	// AWS SSM for Queue URLs.
	GetTargetQueueURL func(string) (string, error)
}

func (p Dispatcher[T]) Dispatch(ctx context.Context, event T, targetQueue string, withDelaySeconds ...int64) (*string, error) {
	traceID, ok := ctx.Value("trace-id").(string)

	log.Println(traceID, ok)
	if !ok {
		return nil, fmt.Errorf("typequeue: missing trace-id")
	}

	queueURL := targetQueue
	if p.GetTargetQueueURL != nil {
		v, err := p.GetTargetQueueURL(targetQueue)

		if err != nil {
			return nil, fmt.Errorf("typequeue: failed to get target queue url: %v", err)
		}

		queueURL = v
	}

	// Marshal the event to JSON
	messageBody, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("typequeue: failed to marshal event: %v", err)
	}

	delay := int64(0)

	if len(withDelaySeconds) > 0 {
		delay = int64(withDelaySeconds[0])
	}

	// Send the message to the target queue
	out, err := p.SQSClient.SendMessage(&sqs.SendMessageInput{
		QueueUrl:     aws.String(queueURL),
		MessageBody:  aws.String(string(messageBody)),
		DelaySeconds: aws.Int64(delay),
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"X-Trace-ID": {
				StringValue: aws.String(traceID),
				DataType:    aws.String("String"),
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("typequeue: failed to dispatch event to SQS: %v", err)
	}

	return out.MessageId, nil
}
