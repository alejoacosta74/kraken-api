package handlers

import (
	"context"

	"github.com/alejoacosta74/kraken-api/internal/kafka"
)

// BaseHandler provides common functionality for all handlers
type BaseHandler struct {
	producerPool kafka.ProducerPool
	ctx          context.Context
	topicName    string
}

// NewBaseHandler creates a new base handler
func NewBaseHandler(ctx context.Context, producerPool kafka.ProducerPool, topicName string) *BaseHandler {
	return &BaseHandler{
		ctx:          ctx,
		producerPool: producerPool,
		topicName:    topicName,
	}
}
