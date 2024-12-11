package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/alejoacosta74/go-logger"
)

// DebugHandler prints received messages to console
type DebugHandler struct {
	logger *logger.Logger
}

// NewDebugHandler creates a new debug handler
func NewDebugHandler() *DebugHandler {
	return &DebugHandler{
		logger: logger.Log,
	}
}

// Handle prints the message in a pretty format
func (h *DebugHandler) Handle(ctx context.Context, msg []byte) error {
	// Pretty print JSON
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, msg, "", "    "); err != nil {
		return fmt.Errorf("error formatting JSON: %w", err)
	}

	h.logger.Info("Received message:\n", prettyJSON.String())
	return nil
}
