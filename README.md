# TypeQueue
A generic, type-safe AWS SQS wrapper for Go that provides both production implementations for dispatching and consuming messages as well as mock implementations for testing.

## Overview
TypeQueue is designed to let you:

- **Dispatch messages** to SQS queues with optional delay and custom trace ID support.
- **Consume messages** from SQS in a type-safe manner with built-in support for JSON unmarshaling and message acknowledgement.
- **Develop and Test seamlessly**: swap in mock implementations for unit tests so you can develop with SQS without ever hitting the real service.

## Features
- `ProductionDispatcher`: Sends messages to SQS queues with built-in JSON marshaling and trace ID propagation.
- `ProductionConsumer`: Consumes SQS messages in a type-strict way, processing messages concurrently and acknowledging them after processing.
- `LambdaConsumer`: Similar to the `ProductionConsumer`, however it exposes `BatchItemFailures` so that only failed events will be retried.
- `MockDispatcher` & `MockConsumer`: Test-friendly implementations that let you simulate SQS behavior locally. Write beautiful unit tests and develop in a live mock environment without incurring AWS costs or using actual AWS credentials.
- Generic & Type-Safe: Leverages Go generics (T any) to ensure compile-time type safety for your message payloads.

## Want to see it in Action?
Check out TypeSend—a robust, type-safe serverless email dispatching system built in Go. It offers scheduling, intuitive template editing (with a ready-to-use UI and endpoints), and extensive customization options.

## Why You'll Love TypeQueue
**Develop Confidently**: With mock implementations that mimic production behavior, you can write comprehensive unit tests and even simulate live message passing—all without ever connecting to real AWS SQS.
**Beautiful Testing**: Our design lets you focus on writing clean, expressive tests. Whether you’re verifying cross-talk between components or simulating delays and retries, TypeQueue makes it easy.
**Cost & Risk-Free**: Avoid AWS costs and security risks during development and testing by using our fully functional mocks.

## Usage

There are a few ways of using this project; however, there is essentially 0 lock-in to the method you choose, this system was built to be easily "plug and play":

### Monolithic Approach
If you'd like a Consumer & Dispatcher within a "monolithic" app, please do the following:

#### Setup the Monolithic Consumer
```
consumer := typequeue.Consumer[*TestMessage]{
	SQSClient: INSERT_YOUR_SQS_CLIENT_HERE,
	Logger:    logrus.New() // or a different logger; this is an interface,
	GetTargetQueueURL: func(target string) (string, error) {
	  // This is super useful if your SQS queues utilize SSM for Queue URLs.
		return target, nil
	},
}

ctx, cancel := context.WithCancel(context.Background())
defer cancel() // or pass this to your "shutdown" function.

// Run consumer in a goroutine.
go consumer.Consume(ctx, typequeue.ConsumerSQSOptions{
		TargetQueue:         "example-queue",
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     10,
	}, func(received *TestMessage) error {
		// Use the Test Message!
		// Return an error IF you want the message to be retried
		// Return `nil` if you want to "acknowledge" (delete) the message
		return nil
	})
```

#### Establish a Dispatcher

```
dispatcher := typequeue.Dispatcher[*TestMessage]{
	SQSClient: INSERT_YOUR_SQS_CLIENT_HERE,
	GetTargetQueueURL: func(target string) (string, error) {
		return target, nil
	},
}
```

Once you have the Dispatcher, you should pass it around to other functions using the `typequeue.TypeQueueDispatcher[T]` interface. **This is critical to make testing easy!**

This same Dispatcher can be used as many times as you'd like, however it is Type-Strict!

```
func doSomething(dispatcher typequeue.TypeQueueDispatcher[*TestMessage]) {
	ctx := context.WithValue(context.Background(), "trace-id", "demo-trace-id")
	event := &TestMessage{Message: "Hello, GitHub User!"}

	delay := int64(5) // Optional!
	msgID, err := dispatcher.Dispatch(ctx, event, "example-queue", delay)
	// msgID, err := dispatcher.Dispatch(ctx, event, "example-queue") // No Delay

	... do something else
}
```

#### That's it!

Easy, right?! Next up, read about how to unit test queue-dependent code!

### Unit Testing Queue-Dependent Code

#### Testing Dispatch Code
We'll test the following code:

```
func doSomething(dispatcher typequeue.TypeQueueDispatcher[*TestMessage]) error {
	ctx := context.WithValue(context.Background(), "trace-id", traceID)
	event := &TestMessage{Message: "Hello, GitHub User!"}

	_, err := dispatcher.Dispatch(ctx, event, "example-queue")

	return err
}
```

In your test file, create a `MockDispatcher[T]`:

```
func TestDoSomething(t *testing.T) {
	// Create a dispatcher without a channel.
	dispatcher := typequeue_mocks.MockDispatcher[*TestMessage]{
		Messages: nil,
	}

	// Dispatch a test message.
	testMsg := &TestMessage{Content: "Hello from map"}

	err := doSomething(testMsg)
	assert.NoError(t, err)

	// Verify that the message was added to the map.
	messages, ok := dispatcher.Messages["example-queue"]
	assert.True(t, ok, "expected target queue key to exist in Messages map")
	assert.Equal(t, 1, len(messages), "expected exactly one message in the target queue")
}
```

Very simple :)

### Testing Consumer Code

The consumer functionality has already been "integration tested" with TestContainers (as well as fully unit tested).

Because of this, the only part that needs to be tested is your "processing function": does it throw an error when it needs to? Does it return nil when it needs to?

### Lambda Consumer

Simply replace your general `Consumer` with the Lambda-specific consumer:
```
	consumer := typequeue_lambda.LambdaConsumer[*TestEvent]{
		Logger:    logger,
		SQSEvents: sqsEvent,
	}
```

Check out the demo code in `cmd/lambda_consumer_demo` for more details.

> [!WARNING]
> **When using in Lambda mode, it is critical that your function runs in "ReportBatchItemFailures" mode.** There is example Terraform code in the above directory as well :)

## Coding without interacting with SQS

The real power of the `MockConsumers/Dispatchers` comes in when you're developing: they have full channel support. This means that you can code (and run) your program "live", without actually hitting SQS. This can save valuable time and money.

Check out [Cross Talk Tests](./pkg/mocked/tests/crosstalk_test.go) to learn how to set this up; it's super easy.

You can also use `build tags` to say like "stub-sqs" to make this easy to turn on / off!

## Support this Project

Firstly, a Star is always appreciated!

PR's are also always welcome :) Please continue to make sure that unit tests pass, as well as integration tests via TestContainers.
