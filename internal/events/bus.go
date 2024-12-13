package events

import (
	"sync"

	"github.com/alejoacosta74/go-logger"
)

// Bus defines the interface for event bus operations
type Bus interface {
	// Publish sends an event to all subscribers of the specified topic
	Publish(topic string, event interface{})
	// Subscribe returns a channel that receives events for the specified topic
	Subscribe(topic string) <-chan interface{}
	// Unsubscribe removes a subscriber channel from the specified topic
	Unsubscribe(topic string, ch <-chan interface{})
}

// EventBus implements the Bus interface providing a concurrent-safe
// publish-subscribe message bus.
type EventBus struct {
	// subscribers maps topics to a set of subscriber channels
	// map[topic]map[chan<- interface{}]struct{} creates a set-like structure
	// where the empty struct{} uses no additional memory
	subscribers map[string]map[chan interface{}]struct{}

	// subscribersMu protects concurrent access to the subscribers map
	// This mutex must be held when modifying the map or its contents
	subscribersMu sync.RWMutex

	// channelBufferSize determines the buffer size for new subscriber channels
	// A buffered channel helps prevent blocking when publishing events
	channelBufferSize int

	// shutdownCh is closed when the event bus is shutting down
	// All subscriber goroutines monitor this channel to clean up
	shutdownCh chan struct{}

	logger *logger.Logger
}

// NewEventBus creates a new EventBus instance.
// The channelBufferSize parameter determines the buffer size for subscriber channels.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers:       make(map[string]map[chan interface{}]struct{}),
		channelBufferSize: 100, // Buffer up to 100 events per subscriber
		shutdownCh:        make(chan struct{}),
		logger:            logger.WithField("component", "event_bus"),
	}
}

// Publish sends an event to all subscribers of the specified topic.
// This method is concurrent-safe and non-blocking.
//
// Parameters:
//   - topic: The topic to publish to
//   - event: The event data to send to subscribers
//
// If a subscriber's channel is full, the event will be dropped for that subscriber.
func (b *EventBus) Publish(topic string, event interface{}) {
	b.subscribersMu.RLock()
	defer b.subscribersMu.RUnlock()

	// Get the subscriber channels for this topic
	subscribers, exists := b.subscribers[topic]
	if !exists {
		b.logger.Trace("No subscribers for this topic")
		return // No subscribers for this topic
	}
	b.logger.Tracef("Publishing received event length  to topic %s subscribers", topic)
	// Send to each subscriber non-blocking
	for subscriberCh := range subscribers {
		select {
		case subscriberCh <- event:
			// Event sent successfully
		default:
			// Channel full, drop event for this subscriber
			b.logger.Warn("Channel full, dropping event for this subscriber")
		}
	}
}

// Subscribe creates a new subscription to the specified topic.
// Returns a channel that will receive events published to the topic.
//
// Parameters:
//   - topic: The topic to subscribe to
//
// Returns:
//   - A receive-only channel for events
//
// The returned channel is buffered with size channelBufferSize.
// The subscriber should always call Unsubscribe when done to prevent resource leaks.
func (b *EventBus) Subscribe(topic string) <-chan interface{} {
	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()

	// Create a new buffered channel for this subscriber
	ch := make(chan interface{}, b.channelBufferSize)

	// Initialize topic subscribers map if it doesn't exist
	if b.subscribers[topic] == nil {
		b.subscribers[topic] = make(map[chan interface{}]struct{})
	}

	// Add the channel to the subscribers map
	b.subscribers[topic][ch] = struct{}{}
	b.logger.Debugf("Subscribed to topic %s", topic)
	return ch
}

// Unsubscribe removes a subscriber from the specified topic.
// This method is concurrent-safe and idempotent.
//
// Parameters:
//   - topic: The topic to unsubscribe from
//   - ch: The channel to unsubscribe (receive-only channel from Subscribe)
//
// Usage example:
//
//	ch := eventBus.Subscribe("book_snapshot")
//	defer eventBus.Unsubscribe("book_snapshot", ch)
func (b *EventBus) Unsubscribe(topic string, ch <-chan interface{}) {
	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()

	subscribers, exists := b.subscribers[topic]
	if !exists {
		b.logger.Trace("No subscribers for this topic")
		return
	}

	// Find and remove the channel from subscribers
	for subCh := range subscribers {
		// Compare channel pointers
		if ch == subCh {
			delete(subscribers, subCh)
			close(subCh)
			b.logger.Tracef("Unsubscribed from topic %s", topic)
			break
		}
	}

	// Clean up topic if no more subscribers
	if len(subscribers) == 0 {
		b.logger.Tracef("No more subscribers for topic %s, removing", topic)
		delete(b.subscribers, topic)
	}
}

// Shutdown gracefully shuts down the event bus.
// It closes all subscriber channels and cleans up resources.
func (b *EventBus) Shutdown() {
	b.logger.Debug("Shutting down event bus")
	// Signal shutdown
	close(b.shutdownCh)

	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()

	// Close all subscriber channels
	for topic, subscribers := range b.subscribers {
		for ch := range subscribers {
			close(ch)
		}
		delete(b.subscribers, topic)
	}
	b.logger.Debug("Event bus shutdown complete")
}

// TopicSubscriberCount returns the number of subscribers for a topic.
// This method is useful for testing and monitoring.
func (b *EventBus) TopicSubscriberCount(topic string) int {
	b.subscribersMu.RLock()
	defer b.subscribersMu.RUnlock()

	return len(b.subscribers[topic])
}
