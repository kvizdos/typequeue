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
- `MockDispatcher` & `MockConsumer`: Test-friendly implementations that let you simulate SQS behavior locally. Write beautiful unit tests and develop in a live mock environment without incurring AWS costs or using actual AWS credentials.
- Generic & Type-Safe: Leverages Go generics (T any) to ensure compile-time type safety for your message payloads.

## Want to see it in Action?
Check out TypeSend—a robust, type-safe serverless email dispatching system built in Go. It offers scheduling, intuitive template editing (with a ready-to-use UI and endpoints), and extensive customization options.

## Why You'll Love TypeQueue
**Develop Confidently**: With mock implementations that mimic production behavior, you can write comprehensive unit tests and even simulate live message passing—all without ever connecting to real AWS SQS.
**Beautiful Testing**: Our design lets you focus on writing clean, expressive tests. Whether you’re verifying cross-talk between components or simulating delays and retries, TypeQueue makes it easy.
**Cost & Risk-Free**: Avoid AWS costs and security risks during development and testing by using our fully functional mocks.

Get started with TypeQueue and develop your SQS-driven applications in a safer, more efficient, and cost-effective way!
