package typequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
)

type batchedMessage struct {
	Body    *string
	TraceID *string
	Delay   *int64
}

type BatchedDispatcher[T SQSAbleMessage] struct {
	SQSClient SQSAPI
	// Can be used to easily incorporate
	// AWS SSM for Queue URLs.
	GetTargetQueueURL func(string) (string, error)
	Logger            Logger

	doneContext context.Context
	doneCancel  context.CancelFunc
	doneWg      sync.WaitGroup

	deliverWg   sync.WaitGroup
	itemChan    chan *batchedMessage
	batchHolder []*batchedMessage
	batchPool   *sync.Pool

	mu            sync.Mutex
	queueURL      *string
	queueURLOnce  sync.Once
	dispatchingWg sync.WaitGroup
	networkPool   chan struct{}
}

func (p *BatchedDispatcher[T]) Flush() {
	p.dispatchingWg.Wait()
	p.doneCancel()
	p.doneWg.Wait()
}

func (p *BatchedDispatcher[T]) Run() {
	// Launch a goroutine that waits for cancellation and then closes itemChan.
	go func() {
		<-p.doneContext.Done()
		close(p.itemChan)
	}()

	for item := range p.itemChan {
		p.mu.Lock()
		p.batchHolder = append(p.batchHolder, item)
		if len(p.batchHolder) == 10 {
			// Swap out the full batch with a new slice from the pool.
			batchToDeliver := p.batchHolder
			p.batchHolder = p.batchPool.Get().([]*batchedMessage)
			p.mu.Unlock()
			p.deliverWg.Add(1)
			go func(batch []*batchedMessage) {
				// Process the batch.
				p.deliverItems(batch)
				// Reset slice before putting it back.
				batch = batch[:0]
				p.batchPool.Put(batch)
			}(batchToDeliver)
		} else {
			p.mu.Unlock()
		}
	}

	// Flush any remaining messages.
	p.mu.Lock()
	if len(p.batchHolder) > 0 {
		batchToDeliver := p.batchHolder
		p.batchHolder = p.batchPool.Get().([]*batchedMessage)
		p.mu.Unlock()
		p.deliverWg.Add(1)
		go func(batch []*batchedMessage) {
			p.deliverItems(batch)
			batch = batch[:0]
			p.batchPool.Put(batch)
		}(batchToDeliver)
	} else {
		p.mu.Unlock()
	}
	p.deliverWg.Wait()
	p.doneWg.Done()
}

func (p *BatchedDispatcher[T]) deliverItems(items []*batchedMessage) {
	p.networkPool <- struct{}{}
	defer func() {
		<-p.networkPool
		p.deliverWg.Done()
	}()

	entries := make([]*sqs.SendMessageBatchRequestEntry, len(items))

	for i, entry := range items {
		entries[i] = &sqs.SendMessageBatchRequestEntry{
			DelaySeconds: entry.Delay,
			Id:           aws.String(uuid.NewString()),
			MessageBody:  entry.Body,
			MessageAttributes: map[string]*sqs.MessageAttributeValue{
				"X-Trace-ID": {
					StringValue: entry.TraceID,
					DataType:    aws.String("String"),
				},
			},
		}
	}

	// Send the message to the target queue
	out, err := p.SQSClient.SendMessageBatch(&sqs.SendMessageBatchInput{
		QueueUrl: p.queueURL,
		Entries:  entries,
	})

	if err != nil {
		p.Logger.Errorf("typequeue: failed to deliverItems: %s", err.Error())
		return
	}

	if len(out.Failed) > 0 {
		for _, failed := range out.Failed {
			p.Logger.Errorf("typequeue: failed to deliverItem (%s): %s. Your fault: %v", *failed.Message, *failed.Code, failed.SenderFault)
		}
	}

	return
}

func (p *BatchedDispatcher[T]) Dispatch(ctx context.Context, event T, targetQueue string, withDelaySeconds ...int64) (*string, error) {
	p.dispatchingWg.Add(1)
	defer p.dispatchingWg.Done()
	select {
	case <-p.doneContext.Done():
		return nil, fmt.Errorf("typequeue: dispatcher has been flushed")
	default:
		// continue and send to p.itemChan
	}

	traceID, ok := ctx.Value("trace-id").(string)

	if !ok {
		return nil, fmt.Errorf("typequeue: missing trace-id")
	}

	var urlErr error
	p.queueURLOnce.Do(func() {
		if p.GetTargetQueueURL != nil {
			var v string
			v, urlErr = p.GetTargetQueueURL(targetQueue)
			if urlErr == nil {
				p.queueURL = aws.String(v)
			}
		} else {
			p.queueURL = aws.String(targetQueue)
		}
	})
	if urlErr != nil {
		return nil, fmt.Errorf("typequeue: failed to get target queue url: %v", urlErr)
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

	p.itemChan <- &batchedMessage{Body: aws.String(string(messageBody)), TraceID: &traceID, Delay: &delay}

	return nil, nil
}

func NewBatchedDispatcher[T SQSAbleMessage](
	parentContext context.Context,
	logger Logger,
	sqsClient SQSAPI,
	getTargetQueueURL func(string) (string, error),
	bufferSize int,
	maxConcurrentNetwork int,
) *BatchedDispatcher[T] {
	if logger == nil {
		panic("NewBatchedDispatcher requires a logger!")
	}

	ctx, cancel := context.WithCancel(parentContext)
	dispatcher := &BatchedDispatcher[T]{
		SQSClient:         sqsClient,
		GetTargetQueueURL: getTargetQueueURL,
		doneContext:       ctx,
		doneCancel:        cancel,
		networkPool:       make(chan struct{}, maxConcurrentNetwork),
		itemChan:          make(chan *batchedMessage, bufferSize),
		Logger:            logger,
	}
	dispatcher.batchPool = &sync.Pool{
		New: func() any {
			// Preallocate with capacity 10.
			return make([]*batchedMessage, 0, 10)
		},
	}
	// Initialize batchHolder from the pool.
	dispatcher.batchHolder = dispatcher.batchPool.Get().([]*batchedMessage)

	dispatcher.doneWg.Add(1)
	go dispatcher.Run()
	return dispatcher
}
