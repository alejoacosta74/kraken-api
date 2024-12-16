package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/common"
	"github.com/alejoacosta74/kraken-api/internal/events"
	"github.com/alejoacosta74/kraken-api/internal/kafka"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

const (
	KafkaProducerPoolSize = 3
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
	handlers map[common.MessageType]MessageHandler

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

	doneChan chan struct{} // Channel to signal when the dispatcher is done

	producerPool kafka.ProducerPool

	ctx context.Context
}

type DispatcherConfig struct {
	Ctx          context.Context
	MsgChan      chan []byte
	ErrChan      chan error
	DoneChan     chan struct{}
	EventBus     events.Bus
	ProducerPool kafka.ProducerPool
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
func NewDispatcher(cfg DispatcherConfig) *Dispatcher {
	d := &Dispatcher{
		handlers:     make(map[common.MessageType]MessageHandler),
		eventBus:     cfg.EventBus,
		logger:       logger.WithField("component", "dispatcher"),
		msgChan:      cfg.MsgChan,
		errChan:      cfg.ErrChan,
		doneChan:     cfg.DoneChan,
		producerPool: cfg.ProducerPool,
		ctx:          cfg.Ctx,
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
func (d *Dispatcher) RegisterHandler(msgType common.MessageType, handler MessageHandler) {
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
func (d *Dispatcher) Run() {
	d.logger.Info("Starting dispatcher")
	defer func() {
		close(d.errChan)
		close(d.doneChan)
	}()

	// Start the producer pool
	d.producerPool.Start()
	d.logger.Debug("Producer pool started")

	var wg sync.WaitGroup // WaitGroup for in-flight messages
	for {
		select {
		case <-d.ctx.Done():
			d.logger.Trace("Context cancelled, waiting for in-flight messages to complete")
			wg.Wait()
			d.logger.Trace("All in-flight messages completed")
			// Stop the producer pool
			poolDoneChan := make(chan struct{})
			d.logger.Trace("Stopping producer pool")
			go func() {
				d.producerPool.Stop()
				close(poolDoneChan)
			}()

			select {
			case <-poolDoneChan:
				d.logger.Trace("Producer pool stopped successfully")
			case <-time.After(2 * time.Second):
				d.logger.Error("Timeout waiting for producer pool to stop")
				d.errChan <- fmt.Errorf("timeout waiting for producer pool to stop")
			}
			return

		case msg := <-d.msgChan:
			wg.Add(1)
			go func(message []byte) {
				defer wg.Done()
				if err := d.dispatch(message); err != nil {
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

	d.logger.WithFields(logger.Fields{
		"msg_type": msgType,
		"msg_size": len(msg),
	}).Debug("About to handle message")

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
	var err error
	if err = handler.Handle(msg); err != nil {
		select {
		case <-d.ctx.Done():
			d.logger.Trace("Context cancelled, not reporting error")
		default:
			d.errChan <- fmt.Errorf("handler error: %w", err)
		}
	}

	if err == nil {
		select {
		case <-d.ctx.Done():
			d.logger.Trace("Context cancelled, not publishing event to bus")
		default:
			d.eventBus.Publish(common.MessageType(msgType), msg)
			d.logger.WithFields(logger.Fields{
				"msg_type": msgType,
				"msg_size": len(msg),
			}).Trace("Published event to bus")
		}
	}

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
func (d *Dispatcher) determineMessageType(msg kraken.GenericResponse) common.MessageType {
	// Add debug logging
	d.logger.Debug("Received message - Channel:", msg.Channel, "Type:", msg.Type, "Method:", msg.Method)

	messageType := common.MessageType("unknown")

	switch {
	case msg.Channel == "book" && msg.Type == "snapshot":
		messageType = common.TypeBookSnapshot
	case msg.Channel == "book" && msg.Type == "update":
		messageType = common.TypeBookUpdate
	case msg.Channel == "heartbeat":
		messageType = common.TypeHeartbeat
	case msg.Channel == "system":
		messageType = common.TypeSystem
	case msg.Method == "subscribe": // Add handling for subscription responses
		return "subscription_response"
	default:
		d.logger.Debug("Unknown message type:", string(msg.Channel), msg.Type, msg.Method)
		return common.MessageType("unknown")
	}

	return messageType
}
