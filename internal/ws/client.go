package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
	"github.com/gorilla/websocket"
)

// WebSocketClient represents a WebSocket client connection to the Kraken API.
// It manages the connection lifecycle and coordinates message reading and writing.
type WebSocketClient struct {
	url          string          // WebSocket server URL
	conn         *websocket.Conn // Underlying WebSocket connection
	logger       *logger.Logger  // Logger instance
	connMutex    sync.Mutex      // Mutex for thread-safe connection handling
	tradingPair  string          // Trading pair to subscribe to
	msgChan      chan []byte     // Channel for incoming messages
	errChan      chan error      // Channel for error reporting
	writer       *Writer         // Handles writing messages to WebSocket
	reader       *Reader         // Handles reading messages from WebSocket
	shutdownChan chan struct{}   // Signal for graceful shutdown
	done         chan struct{}   // Signals when shutdown is complete
}

// Option defines a function type for configuring the WebSocketClient.
// This follows the functional options pattern for flexible configuration.
type Option func(*WebSocketClient)

// WithTradingPair returns an Option that sets the trading pair for the client.
// The trading pair determines which market data the client will subscribe to.
func WithTradingPair(pair string) Option {
	return func(c *WebSocketClient) {
		c.tradingPair = pair
	}
}

// WithBuffers returns an Option that configures the message and error channels.
// These channels are used for internal communication between components.
func WithBuffers(msgChan chan []byte, errChan chan error) Option {
	return func(c *WebSocketClient) {
		c.msgChan = msgChan
		c.errChan = errChan
	}
}

// NewWebSocketClient creates a new WebSocket client with the given URL and options.
// It initializes the client but does not establish the connection.
func NewWebSocketClient(url string, opts ...Option) *WebSocketClient {
	c := &WebSocketClient{
		url:          url,
		logger:       logger.WithField("component", "ws_client"),
		shutdownChan: make(chan struct{}),
		done:         make(chan struct{}),
	}

	// Apply all provided options
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Run starts the WebSocket client and manages its lifecycle.
// It performs the following tasks:
// 1. Establishes the WebSocket connection
// 2. Initializes reader and writer
// 3. Starts message handling goroutines
// 4. Subscribes to the order book
// 5. Waits for context cancellation
//
// The method blocks until the context is cancelled or an error occurs.
func (c *WebSocketClient) Run(ctx context.Context) error {
	c.logger.Debug("Starting WebSocket client")
	defer close(c.done)

	if err := c.connect(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Initialize reader and writer
	c.writer = NewWriter(c.conn)
	c.reader = NewReader(c.conn, c.msgChan, c.errChan)

	// Subscribe to order book after connection is established
	if err := c.subscribeToOrderBook(); err != nil {
		return fmt.Errorf("subscription failed: %w", err)
	}

	// Start reader and writer with timeouts
	readerDone := make(chan struct{})
	writerDone := make(chan struct{})

	go func() {
		c.reader.Run(ctx)
		close(readerDone)
	}()

	go func() {
		c.writer.Run(ctx)
		close(writerDone)
	}()

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		c.logger.Debug("Context cancelled, initiating shutdown")
	case <-c.shutdownChan:
		c.logger.Debug("Shutdown requested")
	}

	// Perform graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Close the WebSocket connection first
	if err := c.shutdown(); err != nil {
		c.logger.Error("Error during shutdown:", err)
	}

	// Wait for components with timeout
	select {
	case <-shutdownCtx.Done():
		c.logger.Warn("Shutdown timeout reached")
		return fmt.Errorf("shutdown timeout")
	case <-readerDone:
		c.logger.Debug("Reader shutdown complete")
		select {
		case <-shutdownCtx.Done():
			return fmt.Errorf("writer shutdown timeout")
		case <-writerDone:
			c.logger.Debug("Writer shutdown complete")
		}
	}

	return nil
}

// connect establishes the WebSocket connection with thread-safety.
// It uses a mutex to ensure only one connection attempt occurs at a time.
func (c *WebSocketClient) connect() error {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		c.logger.Error("Failed to connect to WebSocket:", err)
		return err
	}
	c.logger.Debug("Connected to WebSocket")

	c.conn = conn
	return nil
}

// shutdown performs a clean shutdown of the WebSocket connection.
// It ensures thread-safe access to the connection during closure.
func (c *WebSocketClient) shutdown() error {
	c.logger.Debug("Shutting down WebSocket client")
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.conn != nil {
		// Close the connection
		c.logger.Trace("Sending close message through ws connection")
		if err := c.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		); err != nil {
			c.logger.Error("Error sending close message:", err)
		}
		c.logger.Debug("WS Connection closed")
		return c.conn.Close()
	}
	return nil
}

// subscribeToOrderBook sends a subscription request for order book updates.
// This will be implemented in the next iteration.
func (c *WebSocketClient) subscribeToOrderBook() error {
	c.logger.Debug("Subscribing to order book")
	// Create subscription message
	sub := kraken.BookRequest{
		Method: "subscribe",
		Params: kraken.BookParams{
			Channel:  "book",
			Symbol:   []string{c.tradingPair},
			Depth:    10,
			Snapshot: true,
		},
	}

	// Marshal subscription message
	msg, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("error marshaling subscription: %w", err)
	}

	// Send subscription request
	c.writer.Write(msg)
	c.logger.Info("Sent subscription request for:", c.tradingPair)

	return nil
}

// Shutdown initiates a graceful shutdown of the client
func (c *WebSocketClient) Shutdown() error {
	c.logger.Debug("Shutting down WebSocket client")
	close(c.shutdownChan)
	// Wait for complete shutdown
	c.logger.Trace("shutdownChan closed. Waiting for shutdown to complete")
	<-c.done
	return nil
}

// Done returns a channel that's closed when shutdown is complete
func (c *WebSocketClient) Done() <-chan struct{} {
	c.logger.Debug("Done channel requested")
	return c.done
}
