package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	typequeue "github.com/kvizdos/typequeue/pkg"
	typequeue_lambda "github.com/kvizdos/typequeue/pkg/lambda"
	"github.com/sirupsen/logrus"
)

type TestEvent struct {
	typequeue.SQSAble
	Content string `json:"content"`
}

func handler(ctx context.Context, sqsEvent events.SQSEvent) (map[string]interface{}, error) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	consumer := typequeue_lambda.LambdaConsumer[*TestEvent]{
		Logger:    logger,
		SQSEvents: sqsEvent,
	}
	consumer.Consume(context.Background(), typequeue.ConsumerSQSOptions{}, func(record *TestEvent) error {
		logger.Info("Consuming Message", "content", record.Content)
		if record.Content == "FAIL ME" {
			return fmt.Errorf("Test Failure")
		}

		return nil
	})
	return consumer.GetBatchItemFailures(), nil
}

func main() {
	lambda.Start(handler)
}
