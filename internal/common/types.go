package common

// MessageType represents different types of messages that can be received from the WebSocket API
type MessageType string

// Predefined message types that can be received from the Kraken WebSocket API
const (
	TypeBookSnapshot MessageType = "book_snapshot" // Full order book snapshot
	TypeBookUpdate   MessageType = "book_update"   // Incremental order book update
	TypeHeartbeat    MessageType = "heartbeat"     // Keep-alive message
	TypeSystem       MessageType = "system"        // System status messages
)
