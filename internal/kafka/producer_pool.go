package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/metrics"
)

// Message represents a message to be sent to Kafka
type Message struct {
	Topic   string
	Payload []byte
	Headers map[string]string
}

// ProducerConfig holds configuration for the producer pool
type ProducerConfig struct {
	BrokerList []string // List of Kafka brokers (i.e. ["localhost:9092"])
	PoolSize   int      // Number of producers in the pool
	// Ctx        context.Context
	// Cancel     context.CancelFunc
	Metrics *metrics.MetricsRecorder
	ErrChan chan error // channel to send errors
}

// producerPool manages a pool of KafkaProducers
type producerPool struct {
	producers chan KafkaProducer // Channel to hold producers (KafkaProducer interface)
	config    ProducerConfig
	logger    *logger.Logger
	wg        sync.WaitGroup
	ctx       context.Context    // Controls pool lifecycle
	cancel    context.CancelFunc // For shutting down the pool
	started   bool               // Track if pool has been started
	mu        sync.RWMutex       // Protects started flag
	metrics   *metrics.MetricsRecorder
	errChan   chan error // channel to send errors
}

// NewProducerPool creates a new pool of Kafka producers
func NewProducerPool(config ProducerConfig) (*producerPool, error) {
	if config.PoolSize <= 0 {
		return nil, fmt.Errorf("pool size must be greater than 0")
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &producerPool{
		producers: make(chan KafkaProducer, config.PoolSize),
		config:    config,
		logger:    logger.WithField("component", "kafka_producer_pool"),
		ctx:       ctx,
		cancel:    cancel,
		metrics:   config.Metrics,
		errChan:   make(chan error, 10),
	}

	return pool, nil
}

// Start initializes the producer pool and creates all producers
func (p *producerPool) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("producer pool already started")
	}

	// Initialize the producer pool
	for i := 0; i < p.config.PoolSize; i++ {
		producer, err := newSaramaProducer(p.config)
		if err != nil {
			// Clean up any producers already created
			p.Stop()
			return fmt.Errorf("failed to create producer %d: %w", i, err)
		}
		p.producers <- producer
	}

	p.started = true
	p.logger.Info("Producer pool started successfully")
	return nil
}

// Stop gracefully shuts down the producer pool
func (p *producerPool) Stop() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return fmt.Errorf("producer pool not started")
	}
	p.mu.Unlock()

	p.logger.Info("Stopping producer pool...")

	// Signal shutdown
	p.cancel()

	// Create a timeout for shutdown
	shutdownTimeout := time.After(10 * time.Second)

	// Create a channel to signal completion of cleanup
	done := make(chan struct{})

	go func() {
		// Close all producers
		var closeErr error
		for i := 0; i < cap(p.producers); i++ {
			select {
			case producer := <-p.producers:
				if err := producer.Close(); err != nil {
					p.logger.WithError(err).Error("Failed to close producer")
					closeErr = err
				}
			case <-time.After(1 * time.Second):
				p.logger.Warn("Timeout waiting for producer to be available for closing")
			}
		}

		// Close the producers channel
		close(p.producers)

		p.mu.Lock()
		p.started = false
		p.mu.Unlock()

		if closeErr != nil {
			p.logger.WithError(closeErr).Error("Errors occurred while closing producers")
		}

		close(done)
	}()

	// Wait for cleanup to complete or timeout
	select {
	case <-done:
		p.logger.Info("Producer pool stopped successfully")
		return nil
	case <-shutdownTimeout:
		return fmt.Errorf("timeout while stopping producer pool")
	}
}

// Send sends a message to Kafka using an available producer from the pool.
// It implements the MessageSender interface.
//
// The method:
// 1. Acquires a producer from the pool channel
// 2. Ensures the producer is returned to the pool using defer
// 3. Sets a timeout context for the send operation
// 4. Sends the message using the producer
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - msg: Message containing topic, payload and headers to send
//
// Returns:
//   - error: If context is cancelled, pool timeout occurs, or send fails
//
// Thread Safety:
// - Safe for concurrent use by multiple goroutines
// - Producers are safely managed via channel operations
//
// Timeouts:
// - 3 second timeout waiting for available producer
// - 5 second timeout for the actual send operation
func (p *producerPool) Send(ctx context.Context, topic string, rawMsg []byte) error {
	start := time.Now()

	// update kafka queue size
	p.metrics.UpdateKafkaQueueSize(float64(len(p.producers)))

	msg := Message{
		Topic:   topic,
		Payload: rawMsg,
	}

	p.wg.Add(1)
	defer p.wg.Done()

	select {
	case producer := <-p.producers:
		defer func() {
			select {
			case <-p.ctx.Done():
				// Pool is shutting down
				producer.Close()
			default:
				p.producers <- producer
			}
		}()

		// Use a separate context for the send operation
		sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := producer.Send(sendCtx, msg); err != nil {
			p.errChan <- err
			p.metrics.RecordKafkaError(err.Error())
			return fmt.Errorf("failed to send message: %w", err)
		}

		duration := time.Since(start)
		p.metrics.RecordKafkaMessageSent(msg.Topic, duration)
		return nil

	case <-ctx.Done():
		p.metrics.RecordKafkaError("context_cancelled")
		return fmt.Errorf("operation cancelled by caller: %w", ctx.Err())

	case <-p.ctx.Done():
		p.metrics.RecordKafkaError("producer_pool_shutdown")
		return fmt.Errorf("producer pool is shutting down")
	}
}

// Close gracefully shuts down all producers in the pool by:
// 1. Retrieving each producer from the pool channel
// 2. Closing each producer's connection to Kafka
// 3. Closing the producers channel
//
// If any producer fails to close cleanly, the error is logged and returned,
// but the method continues closing remaining producers.
//
// Thread Safety:
// - Safe to call concurrently with Send()
// - Should only be called once during shutdown
//
// Returns:
// - error: The first error encountered while closing producers, if any
func (p *producerPool) Close() error {
	var closeErr error
	for i := 0; i < cap(p.producers); i++ {
		producer := <-p.producers
		if err := producer.Close(); err != nil {
			closeErr = err
			p.logger.WithError(err).Error("Failed to close producer")
		}
	}
	close(p.producers)
	return closeErr
}
