package ws

import (
	"context"
	"sync"

	"github.com/gorilla/websocket"
)

// Writer handles writing messages to a WebSocket connection.
// It provides thread-safe message writing and manages its own message queue.
type Writer struct {
	conn      *websocket.Conn // The WebSocket connection to write to
	writeChan chan []byte     // Channel for queuing messages to be sent
	done      chan struct{}   // Channel to signal when writer is shutting down
	mutex     sync.Mutex      // Mutex for thread-safe writing
}

// NewWriter creates a new Writer instance.
// It initializes the write channel with a buffer to prevent blocking
// and a done channel for clean shutdown.
func NewWriter(conn *websocket.Conn) *Writer {
	return &Writer{
		conn:      conn,
		writeChan: make(chan []byte, 100), // Buffer up to 100 messages
		done:      make(chan struct{}),
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
func (w *Writer) Run(ctx context.Context) {
	defer close(w.done)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-w.writeChan:
			w.mutex.Lock()
			err := w.conn.WriteMessage(websocket.TextMessage, msg)
			w.mutex.Unlock()
			if err != nil {
				// TODO: Handle error (will be handled by error channel in next iteration)
				return
			}
		}
	}
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
	case <-w.done:
		return
	}
}
