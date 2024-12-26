package events

import "github.com/alejoacosta74/kraken-api/internal/common"

// Bus defines the interface for event bus operations
type Bus interface {
	// Publish sends an event to all subscribers of the specified topic
	Publish(topic common.MessageType, event interface{})
	// Subscribe returns a channel that receives events for the specified topic
	Subscribe(topic common.MessageType) <-chan interface{}
	// Unsubscribe removes a subscriber channel from the specified topic
	Unsubscribe(topic common.MessageType, ch <-chan interface{})
}
