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
	url             string
	conn            *websocket.Conn
	subscriptions   []string
	logger          *logger.Logger
	connMutex       sync.Mutex
	tradingPair     string
	readyChan       chan struct{}                // channel to signal that the client is ready
	doneChan        chan struct{}                // channel to signal that the client is done
	initErrChan     chan error                   // channel to signal that the client has an error
	eventHandlers   map[EventType][]EventHandler // map of event types to event handlers
	eventsMutex     sync.RWMutex
	shutdownOnce    sync.Once
	shutdown        chan struct{}
	responseWaiters map[string]*responseWaiter // map of waiter unique ID to waiter
	waitersMutex    sync.RWMutex
}

type Option func(*WebSocketClient)

func WithTradingPair(pair string) Option {
	return func(c *WebSocketClient) {
		c.tradingPair = pair
	}
}

func NewWebSocketClient(url string, opts ...Option) *WebSocketClient {
	c := WebSocketClient{
		url:             url,
		subscriptions:   []string{},
		logger:          logger.Log,
		readyChan:       make(chan struct{}),
		doneChan:        make(chan struct{}),
		initErrChan:     make(chan error, 1), // buffered channel to avoid blocking
		eventHandlers:   make(map[EventType][]EventHandler),
		shutdown:        make(chan struct{}),
		responseWaiters: make(map[string]*responseWaiter),
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

	// Wait for system status message before allowing subscriptions
	if err := c.waitForSystemStatus(ctx); err != nil {
		c.initErrChan <- err
		return
	}

	close(c.readyChan) // <-- Signals that the client is ready

	// Subscribe to book channel if trading pair is set
	if c.tradingPair != "" {
		if err := c.subscribeToBook(ctx); err != nil {
			c.logger.Error("Error subscribing to book: " + err.Error())
			return
		}
	}

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

	// clean up the connection
	if c.conn != nil {
		c.cleanUp()
	}

	// wait for the reader to finish
	<-readerDone
	c.logger.Debug("Reader finished, exiting client Run goroutine")
}

// cleanup closes the WebSocket connection
func (c *WebSocketClient) cleanUp() {
	c.shutdownOnce.Do(func() {
		c.logger.Debug("Starting cleanup...")
		close(c.shutdown) // Signal handlers to stop
		c.logger.Trace("shutdown channel closed")
		if c.conn != nil {
			c.logger.Debug("Unsubscribing from book channel")
			// First try to unsubscribe gracefully
			if err := c.unsubscribeFromBook(context.Background()); err != nil {
				c.logger.Errorf("Failed to unsubscribe from book: %v", err)
			}

			c.logger.Warnf("Closing connection to websocket at %s", c.url)
			// Send a close frame to the server
			c.connMutex.Lock()
			defer c.connMutex.Unlock()
			c.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			c.conn.Close()
			c.logger.Debug("Connection closed")
		}
		c.logger.Debug("Cleanup complete")
	})
}

// Subscribe allows external code to subscribe to specific event types
func (c *WebSocketClient) Subscribe(eventType EventType, handler EventHandler) {
	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()
	c.eventHandlers[eventType] = append(c.eventHandlers[eventType], handler)
}

// emit sends an event to all registered handlers
func (c *WebSocketClient) emit(event Event) {
	c.eventsMutex.RLock()
	handlers := c.eventHandlers[event.Type]
	c.eventsMutex.RUnlock()

	// Don't emit events if we're shutting down
	select {
	case <-c.shutdown:
		c.logger.Debug("Shutting down, not emitting event")
		return
	default:
		for _, handler := range handlers {
			handler(event)
		}
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

			var msg kraken.StatusMessage
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
