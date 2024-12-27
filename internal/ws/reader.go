package ws

import (
	"context"
	"sync"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/gorilla/websocket"
)

// Reader handles reading messages from a WebSocket connection.
// It runs in its own goroutine and forwards messages to a channel for processing.
type Reader struct {
	ctx      context.Context
	conn     *websocket.Conn // The WebSocket connection to read from
	mutex    sync.Mutex
	msgChan  chan<- []byte // Channel for forwarding received messages to the dispatcher
	errChan  chan<- error  // Channel for reporting errors to client
	doneChan chan struct{} //Channel for signaling that the reader has shutdown
	logger   *logger.Logger
	wg       *sync.WaitGroup
}

// NewReader creates a new Reader instance.
// Parameters:
//   - conn: The WebSocket connection to read from
//   - msgChan: Channel where received messages will be sent
//   - errChan: Channel where errors will be reported
//   - doneChan: Channel for signaling that the reader has shutdown
//   - wg: WaitGroup for reader
func NewReader(ctx context.Context, conn *websocket.Conn, msgChan chan []byte, errChan chan error, doneChan chan struct{}, wg *sync.WaitGroup) *Reader {
	return &Reader{
		ctx:      ctx,
		conn:     conn,
		msgChan:  msgChan,
		errChan:  errChan,
		doneChan: doneChan,
		logger:   logger.WithField("component", "ws_reader"),
		wg:       wg,
		mutex:    sync.Mutex{},
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
func (r *Reader) Run() {
	r.logger.Debug("Starting reader")
	defer func() {
		close(r.doneChan)
		r.wg.Done()
		r.logger.Debug("Reader shutdown complete")
	}()

	// Create a channel to coordinate shutdown
	readDone := make(chan struct{})

	// Start the read loop in a separate goroutine
	go func() {
		defer close(readDone)
		//! only for debugging
		// ticker := time.NewTicker(5 * time.Second)
		// defer ticker.Stop()
		//! end of debugging
		for {
			r.mutex.Lock()
			_, message, err := r.conn.ReadMessage()
			r.mutex.Unlock()
			if err != nil {
				if r.ctx.Err() != nil {
					// Context was cancelled, exit quietly
					r.logger.Debug("Context cancelled, reader exiting")
					return
				}
				// Only report errors if we're not shutting down
				select {
				case r.errChan <- err:
				case <-r.ctx.Done():
					// Don't send error if we're shutting down
				}
				return
			}

			// Try to send message, but also check for shutdown
			select {
			case <-r.ctx.Done():
				r.logger.Debug("Context cancelled, reader exiting")
				return
			case r.msgChan <- message:
				r.logger.Debug("Message sent to msgChan")
			}
			//! only for debugging
			// case <-ticker.C:
			// 	r.msgChan <- message
			// }
			//! end of debugging
		}
	}()

	// Wait for either context cancellation or read loop completion
	select {
	case <-r.ctx.Done():
		r.logger.Debug("Context cancelled, forcing reader shutdown")
		r.conn.SetReadDeadline(time.Now()) // Force read to unblock
		<-readDone                         // Wait for read loop to exit
		r.logger.Trace("Read loop exited")
	case <-readDone:
		r.logger.Trace("Read loop completed naturally")
	}
}

// Done returns a channel that's closed when the reader has completely shut down
func (r *Reader) Done() <-chan struct{} {
	return r.doneChan
}
