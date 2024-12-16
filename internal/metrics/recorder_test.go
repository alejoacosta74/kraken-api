package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/metrics/mocks"
	"github.com/golang/mock/gomock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func setupTestRegistry() {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
}

func TestRecordMetrics(t *testing.T) {
	// Initialize test registry once
	setupTestRegistry()

	tests := []struct {
		name           string
		snapshotEvent  interface{}
		updateEvent    interface{}
		setupMocks     func(*mocks.MockBus, chan interface{}, chan interface{})
		expectedCounts map[string]float64
	}{
		{
			name:          "successful processing of snapshot and update",
			snapshotEvent: []byte(`{"bs":[["100.5","1.5"]], "as":[["101.5","2.0"]]}`),
			updateEvent:   []byte(`{"bs":[["102.5","1.0"]], "as":[["103.5","2.5"]]}`),
			setupMocks: func(mockBus *mocks.MockBus, snapshotCh, updateCh chan interface{}) {
				// Setup subscription channels
				mockBus.EXPECT().Subscribe("book_snapshot").Return(snapshotCh)
				mockBus.EXPECT().Subscribe("book_update").Return(updateCh)

				// Setup unsubscribe expectations
				mockBus.EXPECT().Unsubscribe("book_snapshot", snapshotCh).Times(1)
				mockBus.EXPECT().Unsubscribe("book_update", updateCh).Times(1)
			},
			expectedCounts: map[string]float64{
				"snapshots_received": 1,
				"updates_received":   1,
			},
		},
		{
			name:          "invalid snapshot message format",
			snapshotEvent: []byte(`invalid json`),
			setupMocks: func(mockBus *mocks.MockBus, snapshotCh, updateCh chan interface{}) {
				mockBus.EXPECT().Subscribe("book_snapshot").Return(snapshotCh)
				mockBus.EXPECT().Subscribe("book_update").Return(updateCh)
				mockBus.EXPECT().Unsubscribe("book_snapshot", snapshotCh).Times(1)
				mockBus.EXPECT().Unsubscribe("book_update", updateCh).Times(1)
			},
			expectedCounts: map[string]float64{
				"snapshots_received": 1, // Counter still increments even with invalid format
			},
		},
		{
			name:          "non-byte message type in snapshot channel",
			snapshotEvent: "string instead of []byte", // This will fail type assertion
			setupMocks: func(mockBus *mocks.MockBus, snapshotCh, updateCh chan interface{}) {
				mockBus.EXPECT().Subscribe("book_snapshot").Return(snapshotCh)
				mockBus.EXPECT().Subscribe("book_update").Return(updateCh)
				mockBus.EXPECT().Unsubscribe("book_snapshot", snapshotCh).Times(1)
				mockBus.EXPECT().Unsubscribe("book_update", updateCh).Times(1)
			},
			expectedCounts: map[string]float64{
				"snapshots_received": 0, // Should not increment
			},
		},
		{
			name:        "non-byte message type in update channel",
			updateEvent: 123, // This will fail type assertion
			setupMocks: func(mockBus *mocks.MockBus, snapshotCh, updateCh chan interface{}) {
				mockBus.EXPECT().Subscribe("book_snapshot").Return(snapshotCh)
				mockBus.EXPECT().Subscribe("book_update").Return(updateCh)
				mockBus.EXPECT().Unsubscribe("book_snapshot", snapshotCh).Times(1)
				mockBus.EXPECT().Unsubscribe("book_update", updateCh).Times(1)
			},
			expectedCounts: map[string]float64{
				"updates_received": 0, // Should not increment
			},
		},
		{
			name: "empty channels with immediate context cancellation",
			setupMocks: func(mockBus *mocks.MockBus, snapshotCh, updateCh chan interface{}) {
				mockBus.EXPECT().Subscribe("book_snapshot").Return(snapshotCh)
				mockBus.EXPECT().Subscribe("book_update").Return(updateCh)
				mockBus.EXPECT().Unsubscribe("book_snapshot", snapshotCh).Times(1)
				mockBus.EXPECT().Unsubscribe("book_update", updateCh).Times(1)
			},
			expectedCounts: map[string]float64{
				"snapshots_received": 0,
				"updates_received":   0,
			},
		},
		{
			name:          "malformed JSON in snapshot with valid price fields",
			snapshotEvent: []byte(`{"bs":[["100.5","1.5"]], "as":[["101.5","2.0"], invalid}`),
			setupMocks: func(mockBus *mocks.MockBus, snapshotCh, updateCh chan interface{}) {
				mockBus.EXPECT().Subscribe("book_snapshot").Return(snapshotCh)
				mockBus.EXPECT().Subscribe("book_update").Return(updateCh)
				mockBus.EXPECT().Unsubscribe("book_snapshot", snapshotCh).Times(1)
				mockBus.EXPECT().Unsubscribe("book_update", updateCh).Times(1)
			},
			expectedCounts: map[string]float64{
				"snapshots_received": 1, // Counter should increment even with invalid JSON
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fresh registry for each test
			setupTestRegistry()

			// comment / uncomment to see logs
			logger.SetLevel("trace")

			// Setup mock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock bus
			mockBus := mocks.NewMockBus(ctrl)

			// Create channels for testing
			snapshotCh := make(chan interface{}, 1)
			updateCh := make(chan interface{}, 1)

			// Setup mocks
			tt.setupMocks(mockBus, snapshotCh, updateCh)

			// Create context
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create recorder
			recorder := NewMetricsRecorder(ctx, mockBus)

			// Start recorder in goroutine
			go recorder.recordMetrics(ctx)

			// Allow goroutine to start
			time.Sleep(500 * time.Millisecond)

			// Send test events
			if tt.snapshotEvent != nil {
				snapshotCh <- tt.snapshotEvent
			}
			if tt.updateEvent != nil {
				updateCh <- tt.updateEvent
			}

			// Allow time for processing
			time.Sleep(500 * time.Millisecond)

			// Cancel context
			cancel()
			// Wait for goroutine to finish
			time.Sleep(100 * time.Millisecond)

			// Verify metrics
			for metricName, expectedValue := range tt.expectedCounts {
				var actualValue float64
				switch metricName {
				case "snapshots_received":
					actualValue = testutil.ToFloat64(recorder.orderBookMetrics.snapshotsReceived)
				case "updates_received":
					actualValue = testutil.ToFloat64(recorder.orderBookMetrics.updatesReceived)
				}
				assert.Equal(t, expectedValue, actualValue, "metric %s", metricName)
			}
		})
	}
}
