package typequeue

import "context"

type TypeQueueDispatcher[T SQSAbleMessage] interface {
	// ctx must have a TraceID
	Dispatch(ctx context.Context, event T, targetQueueParameter string, withDelaySeconds ...int64) (*string, error)
}

type TypeQueueProcessingFunc[T SQSAbleMessage] func(msgs T) error

type TypeQueueConsumer[T SQSAbleMessage] interface {
	Consume(ctx context.Context, opts ConsumerSQSOptions, processFunc TypeQueueProcessingFunc[T]) error
	Ack(receiptID *string) error
	Reject(msgID *string) error
}

type ConsumerSQSOptions struct {
	TargetQueue         string
	MaxNumberOfMessages int64
	WaitTimeSeconds     int64
}

type MsgAckFunc func(*string) error

// Logger is a minimal logging interface.
type Logger interface {
	Debugf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Panicf(format string, args ...interface{})
}
