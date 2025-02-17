package typequeue_test

import "github.com/aws/aws-sdk-go/service/sqs"

// fakeSQSClient implements just enough of the SQS API for our tests.
type fakeSQSClient struct {
	SendMessageFn    func(*sqs.SendMessageInput) (*sqs.SendMessageOutput, error)
	ReceiveMessageFn func(*sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageFn  func(queueURL *string, receiptHandle *string) error
}

func (f *fakeSQSClient) SendMessage(input *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
	return f.SendMessageFn(input)
}

func (f *fakeSQSClient) ReceiveMessage(input *sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error) {
	return f.ReceiveMessageFn(input)
}

func (f *fakeSQSClient) DeleteMessage(input *sqs.DeleteMessageInput) (*sqs.DeleteMessageOutput, error) {
	if err := f.DeleteMessageFn(input.QueueUrl, input.ReceiptHandle); err != nil {
		return nil, err
	}
	return &sqs.DeleteMessageOutput{}, nil
}
