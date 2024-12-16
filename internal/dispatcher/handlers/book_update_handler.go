package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

type BookUpdateHandler struct {
	*BaseHandler
	logger *logger.Logger
}

func NewBookUpdateHandler(base *BaseHandler) *BookUpdateHandler {
	return &BookUpdateHandler{
		BaseHandler: base,
		logger:      logger.WithField("component", "book_update_handler"),
	}
}

func (h *BookUpdateHandler) Handle(msg []byte) error {
	var update kraken.SnapshotUpdate
	if err := json.Unmarshal(msg, &update); err != nil {
		return fmt.Errorf("failed to parse book update: %w", err)
	}

	// Log the update
	h.logger.Tracef("Received book update for: %s", update.Data[0].Symbol)

	// Send to producer pool
	if err := h.producerPool.Send(h.ctx, h.topicName, msg); err != nil {
		return fmt.Errorf("failed to send to kafka: %w", err)
	}
	h.logger.Trace("Book update sent to Kafka")

	return nil
}
