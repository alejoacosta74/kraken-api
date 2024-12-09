package client

import (
	"encoding/json"
	"fmt"
	"websocket-client/kraken"
)

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

func (c *WebSocketClient) handleStatusUpdate(message []byte) error {
	var status kraken.StatusMessage
	if err := json.Unmarshal(message, &status); err != nil {
		return fmt.Errorf("failed to parse status update: %w", err)
	}
	c.logger.Infof("Received status update: %s", status.Data[0].System)
	return nil
}

func (c *WebSocketClient) handleHeartbeat(message []byte) error {
	var heartbeat kraken.HeartbeatMessage
	if err := json.Unmarshal(message, &heartbeat); err != nil {
		return fmt.Errorf("failed to parse heartbeat: %w", err)
	}
	c.logger.Infof("Received heartbeat: %s", heartbeat.Channel)
	return nil
}
