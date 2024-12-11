package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"websocket-client/kraken"

	"github.com/google/uuid"
)

// subscribeToBook sends a subscription request to the Kraken WebSocket API
// to receive order book updates for a specific trading pair.
//
// Parameters:
//   - ctx: Context for cancellation control (currently unused but available for future expansion)
//
// The function creates a subscription request with:
//   - Channel: "book"
//   - Trading pair from client configuration
//   - Depth: 10 levels
//   - Snapshot: true (to receive initial order book state)
//
// Thread safety is ensured using a mutex lock around the WebSocket write operation.
//
// Returns:
//   - error: Any error encountered during the subscription request
func (c *WebSocketClient) subscribeToBook(ctx context.Context) error {

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-c.initErrChan:
		return fmt.Errorf("client initialization failed: %w", err)
	case <-c.readyChan:
		// Client is ready, proceed with subscription
	}

	req := kraken.BookRequest{
		Method: "subscribe",
		Params: kraken.BookParams{
			Channel:  "book",
			Symbol:   []string{c.tradingPair},
			Depth:    10,
			Snapshot: true,
		},
	}

	c.connMutex.Lock()
	c.logger.Debugf("Subscribing to book for %s", c.tradingPair)
	err := c.conn.WriteJSON(req)
	c.logger.Debugf("Subscription sent for channel %s and symbol %s", req.Params.Channel, req.Params.Symbol)
	c.connMutex.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send book subscription: %w", err)
	}

	return nil
}

// unsubscribeFromBook sends an unsubscribe message and waits for acknowledgment
func (c *WebSocketClient) unsubscribeFromBook(ctx context.Context) error {
	unsubMsg := kraken.BookUnsubscribe{
		Method: "unsubscribe",
		Params: kraken.BookParams{
			Channel: "book",
			Symbol:  []string{c.tradingPair},
		},
	}

	// Send unsubscribe message
	if err := c.sendMessage(ctx, unsubMsg); err != nil {
		return fmt.Errorf("failed to send unsubscribe message: %w", err)
	}
	c.logger.Debugf("Unsubscribe message sent for channel '%s' and symbol(s) %s", unsubMsg.Params.Channel, unsubMsg.Params.Symbol)

	// Wait for unsubscribe acknowledgment
	var ack kraken.BookUnsubscribeAck
	err := c.waitForResponse(
		ctx,
		func(msg []byte) bool {
			var resp kraken.BookUnsubscribeAck
			if err := json.Unmarshal(msg, &resp); err != nil {
				return false
			}
			return resp.Method == "unsubscribe" && resp.Success
		},
		&ack,
	)
	if err != nil {
		return fmt.Errorf("failed to receive unsubscribe ack: %w", err)
	}

	c.logger.Infof("Successfully unsubscribed from book channel for %s", c.tradingPair)
	return nil
}

// waitForResponse registers a response handler and waits for its completion
func (c *WebSocketClient) waitForResponse(ctx context.Context, matcher MessageMatcher, response interface{}) error {
	// Create a unique ID for this wait
	waitID := uuid.New().String()
	done := make(chan struct{})

	handler := func(msg []byte) error {
		return json.Unmarshal(msg, response)
	}

	// Register the waiter with both matcher and handler
	c.waitersMutex.Lock()
	c.responseWaiters[waitID] = &responseWaiter{
		matcher: matcher, // Use the provided matcher
		handler: handler,
		done:    done,
	}
	c.waitersMutex.Unlock()

	// Cleanup when we're done
	defer func() {
		c.waitersMutex.Lock()
		delete(c.responseWaiters, waitID)
		c.waitersMutex.Unlock()
	}()

	// Wait for either the response or context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	case <-time.After(writeTimeout):
		return fmt.Errorf("timeout waiting for response")
	}
}
