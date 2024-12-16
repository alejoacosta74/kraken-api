package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

// BookSnapshotHandler handles order book snapshot messages
type BookSnapshotHandler struct {
	*BaseHandler
	logger *logger.Logger
}

// NewBookSnapshotHandler creates a new book snapshot handler
func NewBookSnapshotHandler(base *BaseHandler) *BookSnapshotHandler {
	return &BookSnapshotHandler{
		BaseHandler: base,
		logger:      logger.WithField("component", "book_snapshot_handler"),
	}
}

// Handle processes an order book snapshot message
func (h *BookSnapshotHandler) Handle(msg []byte) error {
	var snapshot kraken.BookSnapshot
	if err := json.Unmarshal(msg, &snapshot); err != nil {
		return fmt.Errorf("failed to parse book snapshot: %w", err)
	}

	// Log the snapshot
	h.logger.Trace("Received book snapshot for:", snapshot.Data[0].Symbol)

	// Send to Kafka
	if err := h.producerPool.Send(h.ctx, h.topicName, msg); err != nil {
		return fmt.Errorf("failed to send to kafka: %w", err)
	}
	h.logger.Trace("Book snapshot sent to producer pool")

	return nil
}
