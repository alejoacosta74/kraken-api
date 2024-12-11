package ws

import (
	"context"

	"github.com/gorilla/websocket"
)

// Reader handles reading messages from a WebSocket connection.
// It runs in its own goroutine and forwards messages to a channel for processing.
type Reader struct {
	conn    *websocket.Conn // The WebSocket connection to read from
	msgChan chan []byte     // Channel for forwarding received messages
	errChan chan error      // Channel for reporting errors
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
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// ReadMessage blocks until a message is received
			_, message, err := r.conn.ReadMessage()
			if err != nil {
				r.errChan <- err
				return
			}
			// Forward the message to the processing channel
			r.msgChan <- message
		}
	}
}
