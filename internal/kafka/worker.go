package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alejoacosta74/go-logger"
)

// Worker represents a single worker in the pool.
// Each worker maintains its own Kafka producer connection.
type Worker struct {
	// id is the worker's unique identifier
	id int
	// producer is the underlying Kafka producer
	producer *Producer
	// msgChan receives messages to be sent to Kafka
	msgChan <-chan producerMessage
	// errChan is a channel for reporting errors from the worker
	errChan chan error
	// wg is used to signal when the worker has completed
	wg *sync.WaitGroup
	// logger for worker-specific logging
	logger *logger.Logger
	ctx    context.Context
	// Add a done channel for graceful shutdown
	done chan struct{}
}

// NewWorker creates a new worker with its own Kafka producer.
//
// Parameters:
//   - kafkaClusterAddresses: List of Kafka broker addresses
//   - msgChan: Channel for receiving messages to process
//   - wg: WaitGroup for coordinating shutdown
//
// Returns:
//   - *Worker: A configured worker ready to start
//
// Panics if producer creation fails
func NewWorker(ctx context.Context, kafkaClusterAddresses []string, msgChan <-chan producerMessage, wg *sync.WaitGroup, errChan chan error) *Worker {
	producer, err := NewProducer(kafkaClusterAddresses)
	if err != nil {
		panic(fmt.Sprintf("failed to create producer: %v", err))
	}
	return &Worker{
		producer: producer,
		msgChan:  msgChan,
		wg:       wg,
		errChan:  errChan,
		logger:   logger.WithField("component", "kafka_producer_worker"),
		ctx:      ctx,
		done:     make(chan struct{}),
	}
}

// Start begins the worker's message processing loop.
// The worker will continue until either:
//   - The message channel is closed
//   - The context is cancelled
//
// Parameters:
//   - ctx: Context for cancellation
func (w *Worker) Start(id int) {
	w.id = id
	defer w.Stop()

	for {
		select {
		case <-w.ctx.Done():
			w.logger.Warnf("Context cancelled, stopping worker %d", w.id)
			return
		case msg, ok := <-w.msgChan:
			if !ok {
				w.logger.Warnf("Message channel closed, stopping worker %d", w.id)
				return
			}
			w.logger.WithField("worker_id", w.id).Tracef("Received message to send to Kafka cluster: %+v", msg)
			// Create a timeout channel for the send operation
			sendDone := make(chan error, 1)
			go func() {
				sendDone <- w.producer.SendToKafka(msg.topic, msg.payload)
			}()

			// Wait for either send completion or timeout
			select {
			case err := <-sendDone:
				if err != nil {
					w.logger.Errorf("Failed to send message: %v", err)
					w.errChan <- err
				}
			case <-time.After(2 * time.Second):
				w.logger.WithField("worker_id", w.id).Warnf("Sending message to Kafka cluster timed out after 2 seconds")
				w.errChan <- fmt.Errorf("sending message to Kafka cluster timed out after 2 seconds. worker_id: %d", w.id)
			}
		}
	}
}

// Add a method to wait for worker completion
func (w *Worker) wait() <-chan struct{} {
	return w.done
}

func (w *Worker) Stop() {
	w.logger.Tracef("Closing producer for worker %d", w.id)
	sendDone := make(chan error)
	go func() {
		sendDone <- w.producer.Close()
	}()
	select {
	case err := <-sendDone:
		if err != nil {
			w.logger.Warnf("Error closing producer for worker %d: %v", w.id, err)
		}
	case <-time.After(2 * time.Second):
		w.logger.Warnf("Timeout closing producer for worker %d", w.id)
	}

	w.wg.Done()
	close(w.done)
	w.logger.Debugf("Worker %d finished", w.id)
}
