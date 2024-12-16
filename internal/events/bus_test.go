// bus_test.go
package events

import (
	"sync"
	"testing"
	"time"

	"github.com/alejoacosta74/kraken-api/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestEventBus_Subscribe(t *testing.T) {
	tests := []struct {
		name           string
		topic          common.MessageType
		setupBus       func(*EventBus)                      // Optional setup function
		validateState  func(*testing.T, *EventBus)          // Validation function
		validateOutput func(*testing.T, <-chan interface{}) // Channel validation
	}{
		{
			name:  "subscribe to new topic",
			topic: common.MessageType("test_topic"),
			validateState: func(t *testing.T, b *EventBus) {
				assert.Equal(t, 1, len(b.subscribers))
				assert.Equal(t, 1, b.TopicSubscriberCount(common.MessageType("test_topic")))
			},
			validateOutput: func(t *testing.T, ch <-chan interface{}) {
				assert.NotNil(t, ch)
				assert.Equal(t, 100, cap(ch)) // Verify default buffer size
			},
		},
		{
			name:  "subscribe to existing topic",
			topic: common.MessageType("existing_topic"),
			setupBus: func(b *EventBus) {
				// Pre-subscribe one channel
				b.Subscribe(common.MessageType("existing_topic"))
			},
			validateState: func(t *testing.T, b *EventBus) {
				assert.Equal(t, 1, len(b.subscribers))
				assert.Equal(t, 2, b.TopicSubscriberCount(common.MessageType("existing_topic")))
			},
			validateOutput: func(t *testing.T, ch <-chan interface{}) {
				assert.NotNil(t, ch)
				assert.Equal(t, 100, cap(ch))
			},
		},
		{
			name:  "multiple subscriptions to same topic",
			topic: common.MessageType("multi_topic"),
			setupBus: func(b *EventBus) {
				// Create multiple subscriptions
				for i := 0; i < 3; i++ {
					b.Subscribe(common.MessageType("multi_topic"))
				}
			},
			validateState: func(t *testing.T, b *EventBus) {
				assert.Equal(t, 1, len(b.subscribers))
				assert.Equal(t, 4, b.TopicSubscriberCount(common.MessageType("multi_topic")))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new EventBus for each test
			bus := NewEventBus()

			// Run setup if provided
			if tt.setupBus != nil {
				tt.setupBus(bus)
			}

			// Execute the subscription
			ch := bus.Subscribe(tt.topic)

			// Validate the channel
			if tt.validateOutput != nil {
				tt.validateOutput(t, ch)
			}

			// Validate the EventBus state
			if tt.validateState != nil {
				tt.validateState(t, bus)
			}

			// Test that the channel actually receives messages
			t.Run("channel receives messages", func(t *testing.T) {
				testMessage := "test message"
				go bus.Publish(tt.topic, testMessage)

				select {
				case received := <-ch:
					assert.Equal(t, testMessage, received)
				case <-time.After(100 * time.Millisecond):
					t.Error("timeout waiting for message")
				}
			})
		})
	}
}

func TestEventBus_Publish(t *testing.T) {
	tests := []struct {
		name          string
		topic         common.MessageType
		message       interface{}
		setupBus      func(*EventBus) []<-chan interface{} // Setup function that returns subscriber channels
		expectedSubs  int                                  // Expected number of subscribers
		validateState func(*testing.T, *EventBus)          // Optional state validation
	}{
		{
			name:    "publish to single subscriber",
			topic:   common.MessageType("test_topic"),
			message: "test message",
			setupBus: func(b *EventBus) []<-chan interface{} {
				ch := b.Subscribe(common.MessageType("test_topic"))
				return []<-chan interface{}{ch}
			},
			expectedSubs: 1,
		},
		{
			name:    "publish to multiple subscribers",
			topic:   common.MessageType("multi_topic"),
			message: "broadcast message",
			setupBus: func(b *EventBus) []<-chan interface{} {
				channels := make([]<-chan interface{}, 3)
				for i := 0; i < 3; i++ {
					channels[i] = b.Subscribe(common.MessageType("multi_topic"))
				}
				return channels
			},
			expectedSubs: 3,
		},
		{
			name:    "publish to non-existent topic",
			topic:   common.MessageType("nonexistent"),
			message: "void message",
			setupBus: func(b *EventBus) []<-chan interface{} {
				// Subscribe to a different topic
				ch := b.Subscribe(common.MessageType("other_topic"))
				return []<-chan interface{}{ch}
			},
			expectedSubs: 0,
			validateState: func(t *testing.T, b *EventBus) {
				assert.Equal(t, 0, b.TopicSubscriberCount(common.MessageType("nonexistent")))
			},
		},
		{
			name:    "publish different types of messages",
			topic:   common.MessageType("mixed_types"),
			message: struct{ Data string }{Data: "structured data"},
			setupBus: func(b *EventBus) []<-chan interface{} {
				ch := b.Subscribe(common.MessageType("mixed_types"))
				return []<-chan interface{}{ch}
			},
			expectedSubs: 1,
		},
		{
			name:    "publish nil message",
			topic:   common.MessageType("nil_topic"),
			message: nil,
			setupBus: func(b *EventBus) []<-chan interface{} {
				ch := b.Subscribe(common.MessageType("nil_topic"))
				return []<-chan interface{}{ch}
			},
			expectedSubs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new EventBus for each test
			bus := NewEventBus()

			// Setup subscribers and get channels
			channels := tt.setupBus(bus)

			// Create a WaitGroup to synchronize message reception
			var wg sync.WaitGroup
			wg.Add(tt.expectedSubs)

			// Set up message reception for each subscriber
			receivedMessages := make([]interface{}, len(channels))
			for i, ch := range channels {
				i := i // Create new variable for goroutine
				ch := ch
				go func() {
					select {
					case msg := <-ch:
						receivedMessages[i] = msg
						wg.Done()
					case <-time.After(100 * time.Millisecond):
						// If we don't expect messages for this subscriber, mark as done
						if tt.expectedSubs == 0 {
							wg.Done()
						} else {
							t.Errorf("timeout waiting for message on channel %d", i)
						}
					}
				}()
			}

			// Publish the message
			bus.Publish(tt.topic, tt.message)

			// Wait for all expected messages
			wg.Wait()

			// Validate received messages
			for i, msg := range receivedMessages {
				if i < tt.expectedSubs {
					assert.Equal(t, tt.message, msg, "message mismatch for subscriber %d", i)
				}
			}

			// Validate EventBus state if needed
			if tt.validateState != nil {
				tt.validateState(t, bus)
			}
		})
	}
}

// TestEventBus_PublishConcurrent tests concurrent publishing to the EventBus
func TestEventBus_PublishConcurrent(t *testing.T) {
	bus := NewEventBus()
	topic := common.MessageType("concurrent_topic")

	// Create subscribers
	numSubscribers := 5
	numMessages := 100
	channels := make([]<-chan interface{}, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		channels[i] = bus.Subscribe(topic)
	}

	// Create a WaitGroup for publishers and subscribers
	var pubWg sync.WaitGroup
	var subWg sync.WaitGroup
	pubWg.Add(numMessages)
	subWg.Add(numMessages * numSubscribers)

	// Track received messages
	receivedMessages := make(map[int]int)
	var receivedMu sync.Mutex

	// Start subscribers
	for _, ch := range channels {
		// i := i
		ch := ch
		go func() {
			for msg := range ch {
				if val, ok := msg.(int); ok {
					receivedMu.Lock()
					receivedMessages[val]++
					receivedMu.Unlock()
					subWg.Done()
				}
			}
		}()
	}

	// Start publishers
	for i := 0; i < numMessages; i++ {
		i := i
		go func() {
			bus.Publish(topic, i)
			pubWg.Done()
		}()
	}

	// Wait for all publishes and receives
	pubWg.Wait()
	subWg.Wait()

	// Verify results
	receivedMu.Lock()
	defer receivedMu.Unlock()

	assert.Equal(t, numMessages, len(receivedMessages), "should receive all unique messages")
	for i := 0; i < numMessages; i++ {
		assert.Equal(t, numSubscribers, receivedMessages[i],
			"message %d should be received by all subscribers", i)
	}
}
