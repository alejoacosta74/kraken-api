package kafka

import "context"

type MessageSender interface {
	Send(ctx context.Context, topic string, msg []byte) error
}

type PoolController interface {
	Start() error
	Stop() error
}

// ProducerPool defines the interface for a pool of Kafka producers.
// It provides methods to start the pool, send messages, and gracefully stop.
type ProducerPool interface {
	MessageSender
	PoolController
}

// KafkaProducer defines the interface for a single producer
type KafkaProducer interface {
	Send(ctx context.Context, msg Message) error
	Close() error
}
