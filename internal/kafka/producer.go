package kafka

import (
	"context"
	"time"

	"github.com/IBM/sarama"
)

// Producer wraps Sarama's SyncProducer with additional functionality
type Producer struct {
	client     sarama.SyncProducer
	workerPool *WorkerPool
}

// SendMessage sends a message to a Kafka topic.
// It implements our application's producer interface.
func (p *Producer) SendMessage(ctx context.Context, topic string, msg []byte) error {
	// Create Kafka message
	message := &sarama.ProducerMessage{
		Topic:     topic,
		Value:     sarama.ByteEncoder(msg),
		Timestamp: time.Now(),
	}

	// Send message synchronously
	partition, offset, err := p.client.SendMessage(message)
	if err != nil {
		return err
	}

	// For debugging/metrics (we can use this later)
	_ = partition
	_ = offset

	return nil
}

// NewProducer creates a new Kafka producer
func NewProducer(brokers []string) (*Producer, error) {
	// Create Sarama config
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true

	// Create sync producer
	client, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	return &Producer{
		client: client,
	}, nil
}

// Close closes the producer and releases resources
func (p *Producer) Close() error {
	return p.client.Close()
}

type WorkerPool struct {
	// workers []*Worker
	// jobs    chan *ProducerJob
}
