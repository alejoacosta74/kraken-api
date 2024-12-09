package client

import (
	"context"
	"fmt"

	"websocket-client/kraken"
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
