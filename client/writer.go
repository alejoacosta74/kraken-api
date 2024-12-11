package client

import (
	"context"
	"fmt"
	"time"
)

// writeTimeout is the maximum time allowed to write a message to the websocket
const writeTimeout = 5 * time.Second

type MessageMatcher func([]byte) bool

// ResponseHandler is a function that processes a response message
type ResponseHandler func([]byte) error

// responseWaiter represents a pending response wait
type responseWaiter struct {
	matcher MessageMatcher
	handler ResponseHandler
	done    chan struct{}
}

// sendMessage is a generic method to send any message to the websocket
func (c *WebSocketClient) sendMessage(ctx context.Context, msg interface{}) error {
	c.logger.Tracef("Adquiring conn lock for sending message: %+v", msg)

	if c.conn == nil {
		c.logger.Error("Connection is nil, cannot send message")
		return fmt.Errorf("connection is nil")
	}

	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	// Set write deadline
	deadline := time.Now().Add(writeTimeout)
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}
	c.logger.Tracef("Sending message: %+v", msg)
	// Marshal and send the message
	if err := c.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	return nil
}
