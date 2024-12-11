package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

// BookSnapshotHandler handles order book snapshot messages
type BookSnapshotHandler struct {
	*BaseHandler
}

// NewBookSnapshotHandler creates a new book snapshot handler
func NewBookSnapshotHandler(base *BaseHandler) *BookSnapshotHandler {
	return &BookSnapshotHandler{
		BaseHandler: base,
	}
}

// Handle processes an order book snapshot message
func (h *BookSnapshotHandler) Handle(ctx context.Context, msg []byte) error {
	var snapshot kraken.BookSnapshot
	if err := json.Unmarshal(msg, &snapshot); err != nil {
		return fmt.Errorf("failed to parse book snapshot: %w", err)
	}

	// Log the snapshot
	h.logger.Info("Received book snapshot for:", snapshot.Data[0].Symbol)

	// Send to Kafka
	if err := h.producer.SendMessage(ctx, h.topicName, msg); err != nil {
		return fmt.Errorf("failed to send to kafka: %w", err)
	}

	return nil
}
