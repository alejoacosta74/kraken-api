package client

import (
	"encoding/json"
	"fmt"

	"websocket-client/kraken"
)

// routeResponseMessage processes incoming WebSocket messages and routes them to appropriate handlers
// based on their type and content.
//
// Parameters:
//   - message: Raw byte slice containing the WebSocket message
//
// The function handles several types of messages:
//   - Error messages: Logged at error level
//   - Subscription acknowledgments: Parsed as BookResponse and logged
//   - Book snapshots: Routed to handleBookSnapshot for initial order book state
//   - Book updates: Routed to handleBookUpdate for order book changes
//   - Unhandled messages: Logged at debug level
//
// Message routing process:
//  1. First attempts to unmarshal into a GenericResponse to determine message type
//  2. Based on message fields (Error, Method, Channel, Type), routes to specific handlers
//  3. For subscription acknowledgments, parses into BookResponse for detailed logging
//  4. For book messages, delegates to specialized handlers
//
// Thread safety:
//
//	The function is called from the readMessages loop which handles thread safety
//
// Returns:
//   - error: Any error encountered during message parsing or handling
//
// Common errors:
//   - JSON unmarshaling errors
//   - Subscription response parsing errors
//   - Errors from specialized handlers
func (c *WebSocketClient) routeResponseMessage(message []byte) error {

	var genericMsg kraken.GenericResponse

	if err := json.Unmarshal(message, &genericMsg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Log raw message in debug mode
	c.logger.Debugf("Received message: %s", string(message))

	// Route based on message characteristics
	switch {
	case genericMsg.Error != "":
		c.logger.Error("Received error message: " + genericMsg.Error)
		return nil

	case genericMsg.Method == "subscribe" && genericMsg.Success:
		// Just log the acknowledgment, no need to send it anywhere
		var resp kraken.BookResponse
		if err := json.Unmarshal(message, &resp); err != nil {
			return fmt.Errorf("failed to parse subscription response: %w", err)
		}
		c.logger.Infof("Subscription acknowledged for %s", resp.Result.Symbol)
		return nil

	case genericMsg.Channel == "book" && genericMsg.Type == "snapshot":
		return c.handleBookSnapshot(message)

	case genericMsg.Channel == "book":
		return c.handleBookUpdate(message)

	default:
		c.logger.Debugf("Unhandled message type: %s", string(message))
		return nil
	}
}

func (c *WebSocketClient) handleBookSnapshot(message []byte) error {
	var snapshot kraken.BookSnapshot
	if err := json.Unmarshal(message, &snapshot); err != nil {
		return fmt.Errorf("failed to parse book snapshot: %w", err)
	}

	c.logger.Infof("Received book snapshot for %s", snapshot.Data[0].Symbol)
	c.bookSnapshots <- snapshot
	return nil
}

func (c *WebSocketClient) handleBookUpdate(message []byte) error {
	// Handle book updates if needed
	return nil
}
