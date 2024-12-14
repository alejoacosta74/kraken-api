package ws

import (
	"context"
	"sync"

	"github.com/alejoacosta74/go-logger"
	"github.com/gorilla/websocket"
)

// Writer handles writing messages to a WebSocket connection.
// It provides thread-safe message writing and manages its own message queue.
type Writer struct {
	ctx       context.Context
	conn      *websocket.Conn // The WebSocket connection to write to
	writeChan chan []byte     // Channel for queuing messages to be sent
	doneChan  chan struct{}   // Channel to signal when writer is shutting down
	errChan   chan<- error    // Channel for reporting errors to client
	stopChan  chan struct{}   // Channel to receive request to stop
	mutex     sync.Mutex      // Mutex for thread-safe writing
	logger    *logger.Logger
	wg        *sync.WaitGroup
}

// NewWriter creates a new Writer instance.
// It initializes the write channel with a buffer to prevent blocking
// and a done channel for clean shutdown.
func NewWriter(ctx context.Context, conn *websocket.Conn, errChan chan error, doneChan chan struct{}, stopChan chan struct{}, wg *sync.WaitGroup) *Writer {
	return &Writer{
		conn:      conn,
		writeChan: make(chan []byte, 100), // Buffer up to 100 messages
		doneChan:  doneChan,
		errChan:   errChan,
		stopChan:  stopChan,
		logger:    logger.WithField("component", "ws_writer"),
		ctx:       ctx,
		mutex:     sync.Mutex{},
		wg:        wg,
	}
}

// Run starts the writer's main loop.
// It performs the following tasks:
// 1. Waits for messages on the write channel
// 2. Writes messages to the WebSocket connection in a thread-safe manner
// 3. Handles shutdown when context is cancelled
//
// The loop can be terminated by:
// - Cancelling the provided context
// - An error occurring during write (will be handled by error channel in next iteration)
//
// Parameters:
//   - ctx: Context for cancellation
func (w *Writer) Run() {
	w.logger.Debug("Starting writer")
	defer func() {
		close(w.doneChan)
		w.wg.Done()
		w.logger.Debug("Writer shutdown complete")
	}()

	writerDone := make(chan struct{})

	go func() {
		defer close(writerDone)
		for {
			select {
			case <-w.ctx.Done():
				w.handleWriterShutdown()
				return
			case <-w.stopChan:
				w.handleWriterShutdown()
				return
			case msg := <-w.writeChan:
				w.mutex.Lock()
				w.logger.Tracef("Writing message to WebSocket: %s", string(msg))
				err := w.conn.WriteMessage(websocket.TextMessage, msg)
				w.mutex.Unlock()
				if err != nil {
					w.logger.Errorf("Error writing message to WebSocket: %v", err)
					// Log error but don't send to error channel during shutdown
					if w.ctx.Err() == nil {
						w.errChan <- err
					}
					return
				}
			}
		}
	}()

	<-writerDone
}

// Write queues a message for sending over the WebSocket connection.
// It is safe to call from multiple goroutines.
//
// The method will return immediately if:
// - The message is successfully queued
// - The writer is shutting down (done channel is closed)
//
// Parameters:
//   - msg: The message to send
func (w *Writer) Write(msg []byte) {
	select {
	case w.writeChan <- msg:
	case <-w.ctx.Done():
		return
	}
}

func (w *Writer) handleWriterShutdown() {
	w.logger.Trace("Draining pending messages")
	for {
		select {
		case <-w.writeChan:
			// Discard remaining messages
		default:
			w.logger.Debug("No more messages to drain, exiting")
			return
		}
	}
}
