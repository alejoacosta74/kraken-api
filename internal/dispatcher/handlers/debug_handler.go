package handlers

import (
	"bytes"
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
		logger: logger.WithField("component", "debug_handler"),
	}
}

// Handle prints the message in a pretty format
func (h *DebugHandler) Handle(msg []byte) error {
	// Pretty print JSON
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, msg, "", "    "); err != nil {
		return fmt.Errorf("error formatting JSON: %w", err)
	}

	h.logger.Trace("Received message:\n", prettyJSON.String())
	return nil
}
