package client

import (
	"errors"

	"github.com/gorilla/websocket"
)

// onMessageHandler is a default handler for messages
func (c *WebSocketClient) onMessageHandler(message []byte) {
	c.prettyPrintJSON(message)
}

// onErrorHandler is a default handler for errors
func (c *WebSocketClient) onError(err error) {
	c.logger.Error("Error: " + err.Error())
}

// onConnectionHandler is a default handler for connection
func (c *WebSocketClient) onConnection() {
	c.logger.Info("Connected to websocket")
}

// onDisconnectionHandler is a default handler for disconnection
func (c *WebSocketClient) onDisconnection(err error) {
	c.logger.Warn("Disconnected from websocket: " + err.Error())
}

// Subscribe sends a subscription message to the websocket
func (c *WebSocketClient) Subscribe(subscription string) error {
	if c.conn == nil {
		c.logger.Error("WebSocket connection not established")
		return errors.New("websocket connection not established")
	}
	err := c.conn.WriteMessage(websocket.TextMessage, []byte(subscription))
	if err != nil {
		c.logger.Error("Error sending subscription message: " + err.Error())
		return err
	}
	c.subscriptions = append(c.subscriptions, subscription)
	c.logger.Infof("Subscribed to %s", subscription)
	return nil
}
