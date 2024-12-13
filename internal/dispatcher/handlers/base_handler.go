package handlers

import (
	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/kafka"
)

// BaseHandler provides common functionality for all handlers
type BaseHandler struct {
	logger       *logger.Logger
	producerPool kafka.ProducerPool
	topicName    string
}

// NewBaseHandler creates a new base handler
func NewBaseHandler(producerPool kafka.ProducerPool, topicName string) *BaseHandler {
	return &BaseHandler{
		logger:       logger.WithField("component", "base_handler"),
		producerPool: producerPool,
		topicName:    topicName,
	}
}
