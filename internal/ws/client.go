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
	msgChan     chan []byte     // Channel for incoming messages from websocket (used by reader)
	errChan     chan error      // Channel for error reporting
	writer      *Writer         // Handles writing messages to WebSocket
	reader      *Reader         // Handles reading messages from WebSocket
	ctx         context.Context // Context for cancellation
	wg          *sync.WaitGroup // WaitGroup for reader and writer
	doneChan    chan struct{}   // Channel to signal when the client is done
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

type ClientConfig struct {
	Ctx      context.Context
	Url      string
	MsgChan  chan []byte
	ErrChan  chan error
	DoneChan chan struct{}
	Opts     []Option
}

// NewWebSocketClient creates a new WebSocket client with the given URL and options.
// It initializes the client but does not establish the connection.
func NewWebSocketClient(cfg ClientConfig) *WebSocketClient {
	c := &WebSocketClient{
		url:      cfg.Url,
		logger:   logger.WithField("component", "ws_client"),
		msgChan:  cfg.MsgChan,
		errChan:  cfg.ErrChan,
		ctx:      cfg.Ctx,
		wg:       &sync.WaitGroup{},
		doneChan: cfg.DoneChan,
	}

	// Apply all provided options
	for _, opt := range cfg.Opts {
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
func (c *WebSocketClient) Run() error {
	defer close(c.doneChan)
	c.logger.Debug("Starting WebSocket client")

	if err := c.connect(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Initialize reader and writer
	writerDoneChan := make(chan struct{})
	writeStopChan := make(chan struct{})
	c.writer = NewWriter(c.ctx, c.conn, c.errChan, writerDoneChan, writeStopChan, c.wg)

	c.wg.Add(1)
	go c.writer.Run()

	// Subscribe to order book after connection is established
	if err := c.subscribeToOrderBook(c.msgChan); err != nil {
		c.logger.Errorf("subscription failed: %v", err)
		// stop the writer
		close(writeStopChan)
		c.wg.Wait()
		c.logger.Debug("Writer shutdown complete. Client shutting down with error")
		return fmt.Errorf("subscription failed: %w", err)
	}

	// create a channel to get notification when the reader has shutdown
	readerDone := make(chan struct{})
	c.reader = NewReader(c.ctx, c.conn, c.msgChan, c.errChan, readerDone, c.wg)
	c.wg.Add(1)
	go c.reader.Run()

MainLoop:
	for {
		select {
		case <-c.ctx.Done():
			c.logger.Debug("Context cancelled, initiating shutdown")
			break MainLoop
		case err := <-c.errChan:
			c.logger.Error("Error received from reader:", err)
		}
	}

	// create a slice of errors to collect all errors during shutdown
	var shutdownErrors []error

	// Perform graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Close the WebSocket connection first
	if err := c.shutdown(); err != nil {
		c.logger.Error("Error during shutdown:", err)
		shutdownErrors = append(shutdownErrors, err)
	}

	// Wait for reader and writer components to shutdown
	exitChan := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(exitChan)
	}()

	// Wait for reader and writer components with timeout
	select {
	case <-shutdownCtx.Done():
		c.logger.Warn("Shutdown timeout reached")
		shutdownErrors = append(shutdownErrors, fmt.Errorf("shutdown timeout"))
	case <-exitChan:
		c.logger.Info("Reader and writer components shutdown complete")
	}

	if len(shutdownErrors) > 0 {
		return fmt.Errorf("shutdown errors: %v", shutdownErrors)
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
func (c *WebSocketClient) subscribeToOrderBook(msgChan chan []byte) error {
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

	// wait for the subscription ACK message to be received
	readerChan := make(chan []byte)
	go func() {
		c.connMutex.Lock()
		defer c.connMutex.Unlock()
		_, msg, err = c.conn.ReadMessage()
		if err != nil {
			c.logger.Error("Error reading message:", err)
			return
		}
		readerChan <- msg
	}()

	select {
	case msg := <-readerChan:
		c.logger.Tracef("Received message: %s", msg)
		var subAck kraken.BookResponse
		if err := json.Unmarshal(msg, &subAck); err != nil {
			return fmt.Errorf("error unmarshalling subscription ACK: %w", err)
		}
		if subAck.Success && subAck.Method == "subscribe" {
			c.logger.Info("Received subscription ACK message:", string(msg))
		} else {
			return fmt.Errorf("subscription failed: %s", subAck.Error)
		}
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for subscription ACK message")
	}

	return nil
}
