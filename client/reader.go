package client

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/gorilla/websocket"
)

// readMessages is the main message reading loop for the WebSocket client.
// It continuously reads messages from the WebSocket connection until the context is cancelled
// or an error occurs.
//
// Parameters:
//   - ctx: Context for cancellation control
//   - readerDone: Channel to signal when the reader has completed
//   - readerErrors: Channel to send any errors encountered during reading
//
// The function handles several types of connection errors:
//   - Normal closure: When the connection is closed normally
//   - Unexpected closure: When the connection is closed abnormally
//   - Timeout errors: Which are handled by continuing to read
//
// When an error occurs, it is sent through the readerErrors channel and the function returns.
// When the context is cancelled, it signals completion through readerDone channel.
func (c *WebSocketClient) readMessages(ctx context.Context, readerDone chan<- struct{}, readerErrors chan<- error) {
	defer close(readerDone)
	defer close(readerErrors)

	for {
		select {
		case <-ctx.Done():
			c.logger.Warn("Context done, exiting readMessages")
			readerDone <- struct{}{}
			return
		default:
			message, err := c.readNextMessage()
			if err != nil {
				// check if the error is due to a closed connection
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					c.logger.Warn("Connection closed normally")
					readerErrors <- err
					return
				}
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.logger.Error("Unexpected close error: " + err.Error())
					readerErrors <- err
					return
				}
				if nErr, ok := err.(net.Error); ok && nErr.Timeout() && nErr.Error() == "i/o timeout" {
					// Timeout error, continue to check context
					c.logger.Debugf("Timeout error, continuing. Error: %v", err)
					continue
				}
				readerErrors <- err
				return
			}
			// Try to determine the message type and route accordingly
			if err := c.routeResponseMessage(message); err != nil {
				c.logger.Error("Error routing message: " + err.Error())
			}

		}
	}
}

// readNextMessage reads a single message from the WebSocket connection.
// It sets a read deadline of 2 seconds for each message read attempt.
//
// The function handles two types of messages:
//   - TextMessage: Processed by the onMessageHandler
//   - PingMessage: Automatically responds with a Pong message
//
// Thread safety is ensured using a mutex lock around connection operations.
//
// Returns:
//   - error: Any error encountered during reading or processing the message
//
// Common errors:
//   - Connection nil error
//   - Network timeout errors
//   - WebSocket protocol errors
func (c *WebSocketClient) readNextMessage() ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("connection is nil")
	}

	_, message, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	return message, nil
}
