package typequeue

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
)

func DeleteMessage(sqsClient SQSAPI, queueURL string, receiptHandler *string) error {
	_, err := sqsClient.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: receiptHandler,
	})

	if err != nil {
		return err
	}

	return nil
}
