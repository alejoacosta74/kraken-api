package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
)

// saramaProducer implements the KafkaProducer interface using Sarama's SyncProducer.
// It provides a thread-safe way to send messages to Kafka topics with the following features:
// - Synchronous message production with acknowledgment from brokers
// - Support for message headers
// - Context-based cancellation and timeouts
// - Graceful shutdown capability
//
// The struct wraps a sarama.SyncProducer which handles the actual Kafka communication
// and provides configuration options like retry policies and required acknowledgments.
type saramaProducer struct {
	producer sarama.SyncProducer
}

// newSaramaProducer creates a new Kafka producer using the Sarama library with the following characteristics:
//
// Configuration:
// - Synchronous producer that waits for acknowledgment from brokers
// - RequiredAcks=WaitForAll ensures message is written to all replicas
// - Automatic retries (max 3 attempts) for transient failures
//
// Thread Safety:
// - Safe for concurrent use by multiple goroutines
// - Uses SyncProducer which provides built-in synchronization
//
// Error Handling:
// - Returns error if producer creation fails
// - Wraps Sarama errors with additional context
//
// Performance Considerations:
// - Synchronous nature trades some latency for reliability
// - Retry mechanism helps handle temporary broker issues
//
// Usage:
// Typically instantiated by the producer pool to create the desired number of producers.
// Each producer maintains its own connection to the Kafka cluster.
func newSaramaProducer(config ProducerConfig) (KafkaProducer, error) {
	// Configure Sarama
	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Return.Errors = true
	saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
	saramaConfig.Producer.Retry.Max = 3

	// Create the Sarama producer
	producer, err := sarama.NewSyncProducer(config.BrokerList, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Sarama producer: %w", err)
	}

	return &saramaProducer{producer: producer}, nil
}

// Send sends a message to the Kafka topic using the Sarama producer.
// It handles message serialization, headers, and context-based operations.
func (p *saramaProducer) Send(ctx context.Context, msg Message) error {
	saramaMsg := &sarama.ProducerMessage{
		Topic: msg.Topic,
		Value: sarama.ByteEncoder(msg.Payload),
	}

	// Add headers if present
	if len(msg.Headers) > 0 {
		var headers []sarama.RecordHeader
		for k, v := range msg.Headers {
			headers = append(headers, sarama.RecordHeader{
				Key:   []byte(k),
				Value: []byte(v),
			})
		}
		saramaMsg.Headers = headers
	}

	// Send with context awareness
	done := make(chan error, 1)
	go func() {
		_, _, err := p.producer.SendMessage(saramaMsg)
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close closes the Sarama producer, releasing all associated resources.
func (p *saramaProducer) Close() error {
	return p.producer.Close()
}
