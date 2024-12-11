package client

// EventType represents different types of websocket events
type EventType string

const (
	EventBookSnapshot EventType = "book_snapshot"
	EventBookUpdate   EventType = "book_update"
	EventStatusUpdate EventType = "status_update"
	EventHeartbeat    EventType = "heartbeat"
)

// Event represents a generic websocket event
type Event struct {
	Type EventType
	Data interface{}
}

// EventHandler represents a function that can handle events
type EventHandler func(Event)
