package typequeue

import (
	"github.com/aws/aws-sdk-go/service/sqs"
)

// SQSAPI defines the subset of the AWS SQS client methods that our code uses.
type SQSAPI interface {
	SendMessage(input *sqs.SendMessageInput) (*sqs.SendMessageOutput, error)
	SendMessageBatch(input *sqs.SendMessageBatchInput) (*sqs.SendMessageBatchOutput, error)
	ReceiveMessage(input *sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(input *sqs.DeleteMessageInput) (*sqs.DeleteMessageOutput, error)
}
