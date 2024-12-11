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

	c.logger.Debugf("Received book snapshot for %s", snapshot.Data[0].Symbol)
	c.emit(Event{
		Type: EventBookSnapshot,
		Data: snapshot,
	})
	return nil
}

func (c *WebSocketClient) handleBookUpdate(message []byte) error {
	var update kraken.SnapshotUpdate
	if err := json.Unmarshal(message, &update); err != nil {
		return fmt.Errorf("failed to parse book update: %w", err)
	}
	c.logger.Debugf("Received book update for %s", update.Data[0].Symbol)
	c.emit(Event{
		Type: EventBookUpdate,
		Data: update,
	})
	return nil
}

func (c *WebSocketClient) handleStatusUpdate(message []byte) error {
	var status kraken.StatusMessage
	if err := json.Unmarshal(message, &status); err != nil {
		return fmt.Errorf("failed to parse status update: %w", err)
	}
	c.logger.Debugf("Received status update: %s", status.Data[0].System)
	return nil
}

func (c *WebSocketClient) handleHeartbeat(message []byte) error {
	var heartbeat kraken.HeartbeatMessage
	if err := json.Unmarshal(message, &heartbeat); err != nil {
		return fmt.Errorf("failed to parse heartbeat: %w", err)
	}
	c.logger.Debugf("Received heartbeat: %s", heartbeat.Channel)
	return nil
}

// See: https://docs.kraken.com/api/docs/websocket-v2/ping
func (c *WebSocketClient) handlePing(message []byte) error {
	var ping kraken.PingResponseMessage
	if err := json.Unmarshal(message, &ping); err != nil {
		return fmt.Errorf("failed to parse ping response: %w", err)
	}
	c.logger.Infof("Received ping response: %s", ping.Method)
	return nil
}
