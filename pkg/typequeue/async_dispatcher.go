package typequeue

import (
	"context"
	"sync"
)

type AsyncDispatcher[T SQSAbleMessage] struct {
	Dispatcher   TypeQueueDispatcher[T]
	TargetQueue  string
	ErrorHandler func(FailedMessage[T])
	WaitGroup    sync.WaitGroup

	killContext   context.Context
	msgChan       chan T
	failedMsgChan chan FailedMessage[T]
}

func (b *AsyncDispatcher[T]) Setup(killCtx context.Context, bufferLen int) chan T {
	b.msgChan = make(chan T, bufferLen)
	b.failedMsgChan = make(chan FailedMessage[T], bufferLen)
	b.killContext = killCtx
	return b.msgChan
}

func (b *AsyncDispatcher[T]) Start() {
	b.WaitGroup.Add(2)
	go b.startDispatchListener()
	go b.startFailedListener()
}

func (b *AsyncDispatcher[T]) Stop() {
	// Signal cancellation so that producers stop writing
	// via the KillContext.
	// Wait for all processing goroutines to complete.
	b.WaitGroup.Wait()

	// Now that no writes occur, it's safe to close channels.
	close(b.msgChan)
	close(b.failedMsgChan)
}

func (b *AsyncDispatcher[T]) startDispatchListener() {
	for {
		select {
		case msg := <-b.msgChan:
			dispatchCtx := context.WithValue(context.Background(), "trace-id", *msg.GetTraceID())
			_, err := b.Dispatcher.Dispatch(dispatchCtx, msg, b.TargetQueue)
			if err == nil {
				continue
			}

			b.failedMsgChan <- FailedMessage[T]{
				Msg:   msg,
				Error: err,
			}
		case <-b.killContext.Done():
			// Drain remaining messages before exiting.
			for {
				select {
				case msg := <-b.msgChan:
					dispatchCtx := context.WithValue(context.Background(), "trace-id", *msg.GetTraceID())
					_, err := b.Dispatcher.Dispatch(dispatchCtx, msg, b.TargetQueue)
					if err != nil {
						b.failedMsgChan <- FailedMessage[T]{
							Msg:   msg,
							Error: err,
						}
					}
				default:
					b.WaitGroup.Done()
					return
				}
			}
		}
	}
}

func (b *AsyncDispatcher[T]) startFailedListener() {
	for {
		select {
		case msg := <-b.failedMsgChan:
			b.ErrorHandler(msg)
		case <-b.killContext.Done():
			// Drain remaining failed messages before exiting.
			for {
				select {
				case msg := <-b.failedMsgChan:
					b.ErrorHandler(msg)
				default:
					b.WaitGroup.Done()
					return
				}
			}
		}
	}
}

type FailedMessage[T SQSAbleMessage] struct {
	Msg   T
	Error error
}
