package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/spf13/viper"
)

type MessageSender interface {
	SendMessage(topic string, msg []byte) error
}

type PoolController interface {
	Start()
	Stop()
}

// ProducerPool defines the interface for a pool of Kafka producers.
// It provides methods to start the pool, send messages, and gracefully stop.
type ProducerPool interface {
	MessageSender
	PoolController
}

// WorkerPool implements the ProducerPool interface.
// It manages a collection of workers that handle message production to Kafka.
type WorkerPool struct {
	// ctx is used for coordinating shutdown across the pool
	ctx context.Context
	// cancel is used to signal workers to stop
	cancel context.CancelFunc
	// msgChan is a buffered channel for distributing messages to workers
	// Channel size = numWorkers * 10 to provide adequate buffering
	msgChan chan producerMessage
	// errChan is a channel for reporting errors from the workers
	errChan chan error
	// numWorkers specifies how many worker goroutines to spawn
	numWorkers int
	// wg tracks active workers for graceful shutdown
	wg *sync.WaitGroup
	// logger for pool-specific logging
	logger  *logger.Logger
	workers []*Worker
}

// producerMessage represents a message to be sent to Kafka.
// It combines the destination topic with the message payload.
type producerMessage struct {
	// topic is the Kafka topic to publish to
	topic string
	// payload is the raw message data
	payload []byte
}

// NewProducerPool creates and initializes a new WorkerPool.
//
// Parameters:
//   - ctx: Context for lifecycle management
//   - numWorkers: Number of worker goroutines to spawn
//
// Returns:
//   - *WorkerPool: A configured but not yet started worker pool
func NewProducerPool(ctx context.Context, numWorkers int, errChan chan error) *WorkerPool {
	ctx, cancel := context.WithCancel(ctx)
	msgChan := make(chan producerMessage, numWorkers*10)
	pool := &WorkerPool{
		ctx:        ctx,
		cancel:     cancel,
		msgChan:    msgChan,
		numWorkers: numWorkers,
		wg:         &sync.WaitGroup{},
		logger:     logger.WithField("component", "kafka_producer_pool"),
		workers:    make([]*Worker, numWorkers),
		errChan:    errChan,
	}

	return pool
}

// Start initializes the worker pool by creating the specified number of workers
// and starting them. Each worker maintains its own Kafka producer connection.
func (p *WorkerPool) Start() {
	kafkaClusterAddresses := viper.GetStringSlice("kafka.cluster.addresses")
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		w := NewWorker(p.ctx, kafkaClusterAddresses, p.msgChan, p.wg, p.errChan)
		p.workers[i] = w
		go w.Start(i)
	}
}

// SendMessage queues a message for delivery to Kafka.
// It implements the ProducerPool interface.
//
// Parameters:
//   - ctx: Context for cancellation
//   - topic: Destination Kafka topic
//   - msg: Message payload
//
// Returns:
//   - error: nil on success, ctx.Err() if context is cancelled
func (p *WorkerPool) SendMessage(topic string, msg []byte) error {
	select {
	case p.msgChan <- producerMessage{topic: topic, payload: msg}:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	case <-time.After(3 * time.Second):
		return fmt.Errorf("timeout waiting for message to be sent")
	}
}

// Stop initiates a graceful shutdown of the worker pool.
// It closes the message channel and waits for all workers to complete.
//
// Returns:
func (p *WorkerPool) Stop() {
	p.logger.Info("Stopping producer pool")
	close(p.msgChan)

	// Wait with timeout for each worker
	timeout := time.After(10 * time.Second)
	done := make(chan struct{})

	go func() {
		for _, worker := range p.workers {
			select {
			case <-worker.wait():
				p.logger.Debugf("Worker %d stopped successfully", worker.id)
			case <-timeout:
				p.logger.Errorf("Timeout waiting for worker %d", worker.id)
				return
			}
		}
		close(done)
	}()

	select {
	case <-done:
		p.logger.Info("All workers finished successfully")
	case <-timeout:
		p.logger.Error("Timeout waiting for workers to finish")
	}
}
