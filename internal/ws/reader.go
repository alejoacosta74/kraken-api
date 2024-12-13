package ws

import (
	"context"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/gorilla/websocket"
)

// Reader handles reading messages from a WebSocket connection.
// It runs in its own goroutine and forwards messages to a channel for processing.
type Reader struct {
	conn    *websocket.Conn // The WebSocket connection to read from
	msgChan chan []byte     // Channel for forwarding received messages
	errChan chan error      // Channel for reporting errors
	done    chan struct{}   // Signal for complete shutdown
	logger  *logger.Logger
}

// NewReader creates a new Reader instance.
// Parameters:
//   - conn: The WebSocket connection to read from
//   - msgChan: Channel where received messages will be sent
//   - errChan: Channel where errors will be reported
func NewReader(conn *websocket.Conn, msgChan chan []byte, errChan chan error) *Reader {
	return &Reader{
		conn:    conn,
		msgChan: msgChan,
		errChan: errChan,
		done:    make(chan struct{}),
		logger:  logger.WithField("component", "ws_reader"),
	}
}

// Run starts the reader's main loop.
// It continuously reads messages from the WebSocket connection and forwards them
// to the message channel. If an error occurs during reading, it's sent to the
// error channel and the loop exits.
//
// The loop can be terminated by cancelling the provided context.
//
// Parameters:
//   - ctx: Context for cancellation
func (r *Reader) Run(ctx context.Context) {
	r.logger.Debug("Starting reader")
	defer func() {
		close(r.done)
		r.logger.Debug("Reader shutdown complete")
	}()

	// Create a channel to coordinate shutdown
	readDone := make(chan struct{})

	// Start the read loop in a separate goroutine
	go func() {
		defer close(readDone)
		for {
			_, message, err := r.conn.ReadMessage()
			if err != nil {
				if ctx.Err() != nil {
					// Context was cancelled, exit quietly
					r.logger.Debug("Context cancelled, reader exiting")
					return
				}
				// Only report errors if we're not shutting down
				select {
				case r.errChan <- err:
					r.logger.Error("Error reading message:", err)
				case <-ctx.Done():
					// Don't send error if we're shutting down
				}
				return
			}

			// Try to send message, but also check for shutdown
			select {
			case <-ctx.Done():
				r.logger.Debug("Context cancelled, reader exiting")
				return
			case r.msgChan <- message:
				r.logger.Debug("Message sent to msgChan")
			}
		}
	}()

	// Wait for either context cancellation or read loop completion
	select {
	case <-ctx.Done():
		r.logger.Debug("Context cancelled, forcing reader shutdown")
		r.conn.SetReadDeadline(time.Now()) // Force read to unblock
		<-readDone                         // Wait for read loop to exit
	case <-readDone:
		r.logger.Debug("Read loop completed naturally")
	}
}

// Done returns a channel that's closed when the reader has completely shut down
func (r *Reader) Done() <-chan struct{} {
	r.logger.Trace("Done channel requested")
	return r.done
}
