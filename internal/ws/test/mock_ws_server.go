package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/gorilla/websocket"
)

// MessageHandler is a function that processes a received message and returns a response
type MessageHandler func([]byte) interface{}

// MockWebSocketServer represents a mock WebSocket server for testing
type MockWebSocketServer struct {
	Server *httptest.Server
	// Connections holds all active websocket connections
	Connections []*websocket.Conn
	// Messages received from clients
	ReceivedMessages [][]byte
	// Messages to send to clients
	MessagesToSend [][]byte
	// Map of message type to handler
	messageHandlers map[string]MessageHandler
	mu              sync.Mutex
	upgrader        websocket.Upgrader
}

// NewMockWebSocketServer creates and starts a new mock WebSocket server
func NewMockWebSocketServer() *MockWebSocketServer {
	mock := &MockWebSocketServer{
		Connections:      make([]*websocket.Conn, 0),
		ReceivedMessages: make([][]byte, 0),
		MessagesToSend:   make([][]byte, 0),
		messageHandlers:  make(map[string]MessageHandler),
		upgrader: websocket.Upgrader{
			// Allow all origins for testing
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	// Create test server
	mock.Server = httptest.NewServer(http.HandlerFunc(mock.handleWebSocket))

	// Convert http:// to ws://
	wsURL := "ws" + mock.Server.URL[4:]
	mock.Server.URL = wsURL

	return mock
}

// handleWebSocket handles incoming WebSocket connections in the mock server
func (m *MockWebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	m.mu.Lock()
	m.Connections = append(m.Connections, conn)
	m.mu.Unlock()

	// Start reading messages from the client
	go m.readMessages(conn)

	// Start sending queued messages to the client
	go m.writeMessages(conn)
}

// readMessages reads messages from the client
func (m *MockWebSocketServer) readMessages(conn *websocket.Conn) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		m.mu.Lock()
		m.ReceivedMessages = append(m.ReceivedMessages, message)
		m.mu.Unlock()
		// Handle message and send response
		msgType := m.determineMessageType(message)
		if handler, ok := m.messageHandlers[msgType]; ok {
			response := handler(message)
			if response != nil {
				err := conn.WriteJSON(response)
				if err != nil {
					return
				}
			}
		}

	}
}

// writeMessages sends queued messages to the client
func (m *MockWebSocketServer) writeMessages(conn *websocket.Conn) {
	for _, msg := range m.MessagesToSend {
		err := conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			return
		}
	}
}

// QueueMessage adds a message to be sent to clients
func (m *MockWebSocketServer) QueueMessage(message []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MessagesToSend = append(m.MessagesToSend, message)
}

// GetReceivedMessages returns all messages received from clients
func (m *MockWebSocketServer) GetReceivedMessages() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ReceivedMessages
}

// Close shuts down the mock server and closes all connections
func (m *MockWebSocketServer) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, conn := range m.Connections {
		conn.Close()
	}
	m.Server.Close()
}

// RegisterHandler registers a handler for a specific message type
func (m *MockWebSocketServer) RegisterHandler(messageType string, handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messageHandlers[messageType] = handler
}

// determineMessageType determines the message type from the received message
func (m *MockWebSocketServer) determineMessageType(message []byte) string {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return "error"
	}
	if method, ok := msg["method"].(string); ok {
		return method
	}

	return "unknown"
}
