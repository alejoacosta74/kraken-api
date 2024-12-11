package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/events"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

// MessageType represents different types of messages that can be received from the WebSocket API
type MessageType string

// Predefined message types that can be received from the Kraken WebSocket API
const (
	TypeBookSnapshot MessageType = "book_snapshot" // Full order book snapshot
	TypeBookUpdate   MessageType = "book_update"   // Incremental order book update
	TypeHeartbeat    MessageType = "heartbeat"     // Keep-alive message
	TypeSystem       MessageType = "system"        // System status messages
)

// MessageHandler defines the interface that all message handlers must implement.
// This allows for a pluggable architecture where new handlers can be easily added.
type MessageHandler interface {
	Handle(ctx context.Context, msg []byte) error
}

// Dispatcher manages the routing of WebSocket messages to appropriate handlers.
// It acts as a central hub that receives messages and routes them to the correct handler
// based on the message type.
type Dispatcher struct {
	// Map of message types to their handlers
	handlers map[MessageType]MessageHandler

	// Event bus for publishing events to interested subscribers
	eventBus events.Bus

	// Logger instance for dispatcher-specific logging
	logger *logger.Logger

	// Channel for receiving messages from the WebSocket reader
	// Sender: WebSocket Reader
	// Receivers: Dispatcher's Run() method
	msgChan chan []byte

	// Channel for reporting errors from handlers and dispatcher
	// Senders: Dispatcher, MessageHandlers
	// Receivers: Main application error handling routine
	errChan chan error

	// Mutex for thread-safe access to the handlers map
	handlerMutex sync.RWMutex
}

// NewDispatcher creates and initializes a new message dispatcher.
//
// Parameters:
//   - msgChan: Channel where the dispatcher receives messages from the WebSocket reader
//   - errChan: Channel for reporting errors to the main application
//   - eventBus: Event bus for publishing events to interested subscribers
//
// Returns:
//   - A configured Dispatcher instance ready to start processing messages
func NewDispatcher(msgChan chan []byte, errChan chan error, eventBus events.Bus) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[MessageType]MessageHandler),
		eventBus: eventBus,
		logger:   logger.Log,
		msgChan:  msgChan,
		errChan:  errChan,
	}
}

// RegisterHandler registers a handler for a specific message type.
// This method is thread-safe and can be called concurrently.
//
// Parameters:
//   - msgType: The type of message this handler should process
//   - handler: The handler implementation for this message type
//
// Usage example:
//
//	dispatcher.RegisterHandler(TypeBookSnapshot, NewBookSnapshotHandler())
func (d *Dispatcher) RegisterHandler(msgType MessageType, handler MessageHandler) {
	d.handlerMutex.Lock()
	defer d.handlerMutex.Unlock()
	d.handlers[msgType] = handler
}

// Run starts the dispatcher's main message processing loop.
// It continuously listens for messages and routes them to appropriate handlers.
//
// The dispatcher will run until:
//   - The context is cancelled
//   - A fatal error occurs
//
// Parameters:
//   - ctx: Context for cancellation and shutdown
//
// Message flow:
//  1. Receive message from msgChan
//  2. Determine message type
//  3. Route to appropriate handler
//  4. Report any errors on errChan
func (d *Dispatcher) Run(ctx context.Context) {
	d.logger.Info("Starting dispatcher")

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("Shutting down dispatcher")
			return

		case msg := <-d.msgChan:
			if err := d.dispatch(ctx, msg); err != nil {
				d.errChan <- fmt.Errorf("dispatch error: %w", err)
			}
		}
	}
}

// dispatch processes a single message by:
// 1. Parsing the message to determine its type
// 2. Finding the appropriate handler
// 3. Executing the handler
// 4. Publishing an event about the processed message
//
// Parameters:
//   - ctx: Context for cancellation
//   - msg: Raw message bytes to process
//
// Returns:
//   - error: Any error that occurred during processing
//
// Message processing flow:
//  1. Parse message → GenericResponse
//  2. Determine message type
//  3. Look up handler
//  4. Execute handler
//  5. Publish event
func (d *Dispatcher) dispatch(ctx context.Context, msg []byte) error {
	// Parse the generic response to determine message type
	var genericMsg kraken.GenericResponse
	if err := json.Unmarshal(msg, &genericMsg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Determine message type
	msgType := d.determineMessageType(genericMsg)

	// Get handler for message type (thread-safe)
	d.handlerMutex.RLock()
	handler, exists := d.handlers[msgType]
	d.handlerMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no handler registered for message type: %s", msgType)
	}

	// Handle the message
	if err := handler.Handle(ctx, msg); err != nil {
		return fmt.Errorf("handler error for message type %s: %w", msgType, err)
	}

	// Publish event about message processing
	// This is now clearly the dispatcher's responsibility
	d.eventBus.Publish(string(msgType), msg)

	return nil
}

// determineMessageType identifies the type of message received based on
// the channel and type fields in the generic response.
//
// Parameters:
//   - msg: The parsed generic response message
//
// Returns:
//   - MessageType: The determined message type or "unknown"
//
// Message type determination rules:
//   - Book snapshot: channel="book" & type="snapshot"
//   - Book update: channel="book" & type="update"
//   - Heartbeat: channel="heartbeat"
//   - System: channel="system"
func (d *Dispatcher) determineMessageType(msg kraken.GenericResponse) MessageType {
	switch {
	case msg.Channel == "book" && msg.Type == "snapshot":
		return TypeBookSnapshot
	case msg.Channel == "book" && msg.Type == "update":
		return TypeBookUpdate
	case msg.Channel == "heartbeat":
		return TypeHeartbeat
	case msg.Channel == "system":
		return TypeSystem
	default:
		return MessageType("unknown")
	}
}
