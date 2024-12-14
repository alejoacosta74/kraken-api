package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/events"
	"github.com/alejoacosta74/kraken-api/internal/kafka"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

const (
	KafkaProducerPoolSize = 3
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
	Handle(msg []byte) error
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
	msgChan <-chan []byte

	// Channel for reporting errors from handlers and dispatcher
	// Senders: Dispatcher, MessageHandlers
	// Receivers: Main application error handling routine
	errChan chan error

	// Mutex for thread-safe access to the handlers map
	handlerMutex sync.RWMutex

	producerPool kafka.ProducerPool
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
func NewDispatcher(ctx context.Context, msgChan chan []byte, errChan chan error, eventBus events.Bus) *Dispatcher {
	producerPool := kafka.NewProducerPool(ctx, KafkaProducerPoolSize)
	d := &Dispatcher{
		handlers:     make(map[MessageType]MessageHandler),
		eventBus:     eventBus,
		logger:       logger.WithField("component", "dispatcher"),
		msgChan:      msgChan,
		errChan:      errChan,
		producerPool: producerPool,
	}

	return d
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
	d.logger.Debug("Registering handler for message type:", msgType)
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
	defer d.logger.Info("Dispatcher shutdown complete")

	// Start the producer pool
	d.producerPool.Start()
	d.logger.Debug("Producer pool started")

	var wg sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			d.logger.Trace("Context cancelled, waiting for in-flight messages to complete")
			wg.Wait()
			d.logger.Trace("All in-flight messages completed")
			// Create a timeout channel for producer pool shutdown
			timeout := time.After(12 * time.Second)
			done := make(chan struct{})
			d.logger.Trace("Stopping producer pool")
			go func() {
				d.producerPool.Stop()
				close(done)
			}()

			select {
			case <-done:
				d.logger.Trace("Producer pool stopped successfully")
			case <-timeout:
				d.logger.Error("Timeout waiting for producer pool to stop")
			}
			return

		case msg := <-d.msgChan:
			wg.Add(1)
			go func(msg []byte) {
				defer wg.Done()
				if err := d.dispatch(ctx, msg); err != nil {
					d.errChan <- fmt.Errorf("dispatch error: %w", err)
				}
			}(msg)
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
func (d *Dispatcher) dispatch(msg []byte) error {
	// Parse the generic response to determine message type
	var genericMsg kraken.GenericResponse
	if err := json.Unmarshal(msg, &genericMsg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Determine message type
	msgType := d.determineMessageType(genericMsg)
	d.logger.Tracef("Message type: %s", msgType)
	// Get handler for message type (thread-safe)
	d.handlerMutex.RLock()
	handler, exists := d.handlers[msgType]
	d.handlerMutex.RUnlock()

	if !exists {
		d.logger.Errorf("No handler registered for message type: %s. Message: %s", msgType, string(msg))
		return fmt.Errorf("no handler registered for message type: %s", msgType)
	}
	d.logger.Tracef("Handler found for message type: %s", msgType)
	// Handle the message
	if err := handler.Handle(msg); err != nil {
		return fmt.Errorf("handler error for message type %s: %w", msgType, err)
	}

	// Publish event about message processing
	// This is now clearly the dispatcher's responsibility
	d.eventBus.Publish(string(msgType), msg)
	d.logger.Tracef("Event published: %s", string(msgType))

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
	// Add debug logging
	d.logger.Debug("Received message - Channel:", msg.Channel, "Type:", msg.Type, "Method:", msg.Method)

	switch {
	case msg.Channel == "book" && msg.Type == "snapshot":
		return TypeBookSnapshot
	case msg.Channel == "book" && msg.Type == "update":
		return TypeBookUpdate
	case msg.Channel == "heartbeat":
		return TypeHeartbeat
	case msg.Channel == "system":
		return TypeSystem
	case msg.Method == "subscribe": // Add handling for subscription responses
		return "subscription_response"
	default:
		d.logger.Debug("Unknown message type:", string(msg.Channel), msg.Type, msg.Method)
		return MessageType("unknown")
	}
}

// GetProducerPool returns the dispatcher's producer pool
func (d *Dispatcher) GetProducerPool() kafka.ProducerPool {
	return d.producerPool
}
