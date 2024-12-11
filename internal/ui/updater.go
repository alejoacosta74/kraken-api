package ui

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/alejoacosta74/kraken-api/internal/events"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

// UIUpdater handles updating the user interface based on received events
type UIUpdater struct {
	eventBus    events.Bus
	currentBook *kraken.BookSnapshot
	mutex       sync.RWMutex
}

// NewUIUpdater creates a new UI updater
func NewUIUpdater(eventBus events.Bus) *UIUpdater {
	return &UIUpdater{
		eventBus: eventBus,
	}
}

// Start begins listening for events to update the UI
func (u *UIUpdater) Start() {
	// Subscribe to book snapshot events
	snapshots := u.eventBus.Subscribe("book_snapshot")
	updates := u.eventBus.Subscribe("book_update")

	// Handle snapshots
	go func() {
		for event := range snapshots {
			if msg, ok := event.([]byte); ok {
				var snapshot kraken.BookSnapshot
				if err := json.Unmarshal(msg, &snapshot); err != nil {
					fmt.Printf("Error parsing snapshot: %v\n", err)
					continue
				}
				u.updateOrderBookDisplay(&snapshot)
			}
		}
	}()

	// Handle updates (to be implemented)
	go func() {
		for range updates {
			// Handle updates
		}
	}()
}

// updateOrderBookDisplay updates the display with new order book data
func (u *UIUpdater) updateOrderBookDisplay(snapshot *kraken.BookSnapshot) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.currentBook = snapshot
	fmt.Println(snapshot.PrettyPrint())
}
