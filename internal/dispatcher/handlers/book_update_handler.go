package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

type BookUpdateHandler struct {
	*BaseHandler
}

func NewBookUpdateHandler(base *BaseHandler) *BookUpdateHandler {
	return &BookUpdateHandler{
		BaseHandler: base,
	}
}

func (h *BookUpdateHandler) Handle(ctx context.Context, msg []byte) error {
	var update kraken.SnapshotUpdate
	if err := json.Unmarshal(msg, &update); err != nil {
		return fmt.Errorf("failed to parse book update: %w", err)
	}

	// Log the update
	h.logger.Trace("Received book update for:", update.Data[0].Symbol)

	// Send to Kafka
	if err := h.producerPool.SendMessage(ctx, h.topicName, msg); err != nil {
		return fmt.Errorf("failed to send to kafka: %w", err)
	}
	h.logger.Trace("Book update sent to Kafka")

	return nil
}
