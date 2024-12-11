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
	url         string          // WebSocket server URL
	conn        *websocket.Conn // Underlying WebSocket connection
	logger      *logger.Logger  // Logger instance
	connMutex   sync.Mutex      // Mutex for thread-safe connection handling
	tradingPair string          // Trading pair to subscribe to
	msgChan     chan []byte     // Channel for incoming messages
	errChan     chan error      // Channel for error reporting
	writer      *Writer         // Handles writing messages to WebSocket
	reader      *Reader         // Handles reading messages from WebSocket
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
		url:    url,
		logger: logger.Log,
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
	if err := c.connect(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Initialize and start reader and writer
	c.writer = NewWriter(c.conn)
	c.reader = NewReader(c.conn, c.msgChan, c.errChan)

	go c.writer.Run(ctx)
	go c.reader.Run(ctx)

	// Subscribe to order book updates
	if err := c.subscribeToOrderBook(); err != nil {
		return fmt.Errorf("subscription failed: %w", err)
	}

	<-ctx.Done() // Wait for context cancellation
	return c.shutdown()
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
		return err
	}

	c.conn = conn
	return nil
}

// shutdown performs a clean shutdown of the WebSocket connection.
// It ensures thread-safe access to the connection during closure.
func (c *WebSocketClient) shutdown() error {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// subscribeToOrderBook sends a subscription request for order book updates.
// This will be implemented in the next iteration.
func (c *WebSocketClient) subscribeToOrderBook() error {
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
