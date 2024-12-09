package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"websocket-client/kraken"

	"github.com/alejoacosta74/go-logger"
	"github.com/gorilla/websocket"
)

type WebSocketClient struct {
	url  string
	conn *websocket.Conn

	subscriptions []string

	logger *logger.Logger

	connMutex     sync.Mutex
	bookSnapshots chan kraken.BookSnapshot // Channel for book snapshots
	tradingPair   string

	readyChan   chan struct{} // signals when client is ready to receive subscriptions
	doneChan    chan struct{} // signals when client has stopped
	initErrChan chan error    // signals initialization errors
}

type Option func(*WebSocketClient)

func WithTradingPair(pair string) Option {
	return func(c *WebSocketClient) {
		c.tradingPair = pair
	}
}

func NewWebSocketClient(url string, opts ...Option) *WebSocketClient {
	c := WebSocketClient{
		url:           url,
		subscriptions: []string{},
		logger:        logger.Log,
		bookSnapshots: make(chan kraken.BookSnapshot),
		readyChan:     make(chan struct{}),
		doneChan:      make(chan struct{}),
		initErrChan:   make(chan error, 1), // buffered channel to avoid blocking
	}

	for _, opt := range opts {
		opt(&c)
	}

	return &c
}

// Run connects and listen for messages
func (c *WebSocketClient) Run(ctx context.Context) {
	defer close(c.doneChan)
	var err error

	// dial the websocket
	c.conn, _, err = websocket.DefaultDialer.Dial(c.url, nil)
	if err != nil {
		c.logger.Error("Error connecting to websocket: " + err.Error())
		c.initErrChan <- err
		return
	}

	// defer c.cleanUp()

	// Wait for system status message before allowing subscriptions
	if err := c.waitForSystemStatus(ctx); err != nil {
		c.initErrChan <- err
		return
	}

	// c.onConnection()

	close(c.readyChan) // <-- Signals that the client is ready

	// add subscription manually for now
	// subscription := "{ \"event\":\"subscribe\", \"subscription\":{\"name\":\"trade\"},\"pair\":[\"ETH/USD\"] }"

	// c.subscriptions = append(c.subscriptions, subscription)
	// c.logger.Infof("Subscribed to %s", subscription)

	// Send subscriptions
	for _, subscription := range c.subscriptions {
		err := c.Subscribe(subscription)
		if err != nil {
			c.logger.Error("Error subscribing to " + subscription + ": " + err.Error())
		}
	}

	// Subscribe to book channel if trading pair is set
	if c.tradingPair != "" {
		if err := c.subscribeToBook(ctx); err != nil {
			c.logger.Error("Error subscribing to book: " + err.Error())
			return
		}
	}
	// subscribe to a book channel if trading pair is set

	// channel to stop the readMessages goroutine
	readerDone := make(chan struct{})
	// channel to receive errors from the reader
	readerErrors := make(chan error)

	go c.readMessages(ctx, readerDone, readerErrors)

	// wait for the context to be done or the reader to finish
	select {
	case <-ctx.Done():
		c.logger.Warn("Shutting down...")
	case err := <-readerErrors:
		c.logger.Error("Reader error: " + err.Error())
	}

	// use a mutex to protect access to the websocket connection while it is being closed
	c.connMutex.Lock()
	if c.conn != nil {
		c.cleanUp()
	}
	c.connMutex.Unlock()

	// wait for the reader to finish
	<-readerDone
}

// cleanup closes the WebSocket connection
func (c *WebSocketClient) cleanUp() {
	if c.conn != nil {
		c.logger.Warnf("Closing connection to websocket at %s", c.url)
		// Send a close frame to the server to initiate a clean websocket shutdown
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
	}
}

// GetBookSnapshot retrieves the next available order book snapshot from the snapshot channel.
//
// Parameters:
//   - ctx: Context for cancellation control
//
// Returns:
//   - *kraken.BookSnapshot: Pointer to the received order book snapshot
//   - error: Context error if the context is cancelled before receiving a snapshot
//
// The function will block until either:
//  1. A snapshot is received from the bookSnapshots channel
//  2. The context is cancelled
func (c *WebSocketClient) GetBookSnapshot(ctx context.Context) (*kraken.BookSnapshot, error) {
	select {
	case snapshot := <-c.bookSnapshots:
		return &snapshot, nil
	case <-c.doneChan:
		return nil, fmt.Errorf("client is shutting down")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// waitForSystemStatus waits for the initial system status message
// that indicates the WebSocket connection is ready
func (c *WebSocketClient) waitForSystemStatus(ctx context.Context) error {
	// Set a reasonable timeout for receiving the status message
	deadline := time.Now().Add(5 * time.Second)
	c.conn.SetReadDeadline(deadline)

	// Read and process messages until we get a status message
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("failed to read status message: %w", err)
			}

			// For v2 API
			var msg struct {
				Channel string `json:"channel"` // "status"
				Type    string `json:"type"`    // "update"
				Data    []struct {
					System       string `json:"system"`
					APIVersion   string `json:"api_version"`
					ConnectionID uint64 `json:"connection_id"`
					Version      string `json:"version"`
				} `json:"data"`
			}
			if err := json.Unmarshal(message, &msg); err != nil {
				c.logger.Debugf("Not a status message: %s, error: %s", string(message), err.Error())
				continue
			}

			// Check if this is a status message
			if msg.Channel == "status" && msg.Type == "update" && len(msg.Data) > 0 {
				if msg.Data[0].System == "online" {
					c.logger.Infof("Connected to Kraken API %s (v%s)", msg.Data[0].APIVersion, msg.Data[0].Version)
					return nil
				}
			}
		}
	}
}
