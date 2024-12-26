package handlers

// MessageHandler defines the interface that all message handlers must implement.
// This allows for a pluggable architecture where new handlers can be easily added.
type MessageHandler interface {
	Handle(msg []byte) error
}
