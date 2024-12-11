package handlers

import (
	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/kafka"
)

// BaseHandler provides common functionality for all handlers
type BaseHandler struct {
	logger    *logger.Logger
	producer  *kafka.Producer
	topicName string
}

// NewBaseHandler creates a new base handler
func NewBaseHandler(producer *kafka.Producer, topicName string) *BaseHandler {
	return &BaseHandler{
		logger:    logger.Log,
		producer:  producer,
		topicName: topicName,
	}
}
