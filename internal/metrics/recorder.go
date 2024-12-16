package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/common"
	"github.com/alejoacosta74/kraken-api/internal/events"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

// MetricsRecorder handles the collection and recording of metrics
type MetricsRecorder struct {
	// Keep the existing metrics definitions
	orderBookMetrics struct {
		snapshotsReceived prometheus.Counter
		updatesReceived   prometheus.Counter
		processLatency    prometheus.Histogram
		snapshotSize      prometheus.Histogram
		priceSpread       prometheus.Gauge
	}
	wsMetrics struct {
		messageReceived  *prometheus.CounterVec
		connectionErrors prometheus.Counter
		messageLatency   prometheus.Histogram
	}
	// kafkaMetrics struct {
	// 	messagesSent   *prometheus.CounterVec
	// 	sendErrors     prometheus.Counter
	// 	messageLatency prometheus.Histogram
	// }

	eventBus events.Bus
	logger   *logger.Logger
	ctx      context.Context
	done     chan struct{}
}

func NewMetricsRecorder(ctx context.Context, eventBus events.Bus) *MetricsRecorder {
	r := &MetricsRecorder{
		eventBus: eventBus,
		logger:   logger.WithField("component", "metrics_recorder"),
		ctx:      ctx,
		done:     make(chan struct{}),
	}

	// Use promauto consistently - it handles registration automatically
	r.wsMetrics.messageReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "ws",             // Add namespace for better organization
			Name:      "messages_total", // Follow Prometheus naming conventions
			Help:      "Number of messages received from WebSocket by type",
		},
		[]string{"type"},
	)

	r.wsMetrics.connectionErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ws",
		Name:      "connection_errors_total",
		Help:      "Total number of WebSocket connection errors",
	})

	r.wsMetrics.messageLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "ws",
		Name:      "message_latency_seconds",
		Help:      "Latency of WebSocket message processing in seconds",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1}, // Define relevant buckets
	})

	// Initialize order book metrics
	r.orderBookMetrics.snapshotsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orderbook",
		Name:      "snapshots_total", // This creates orderbook_snapshots_total query on Prometheus App
		Help:      "Total number of order book snapshots received",
	})

	r.orderBookMetrics.updatesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orderbook",
		Name:      "updates_total", // This creates orderbook_updates_total query on Prometheus App
		Help:      "Total number of order book updates received",
	})

	r.orderBookMetrics.processLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "orderbook",
		Name:      "process_latency_seconds",
		Help:      "Latency of order book processing in seconds",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
	})

	r.orderBookMetrics.snapshotSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "orderbook",
		Name:      "snapshot_size_bytes",
		Help:      "Size of order book snapshots in bytes",
		Buckets:   prometheus.ExponentialBuckets(1024, 2, 10), // From 1KB to 1MB
	})

	r.orderBookMetrics.priceSpread = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "orderbook",
		Name:      "price_spread",
		Help:      "Current spread between best bid and ask prices",
	})

	r.logger.Debug("Metrics recorder initialized")
	return r
}

// Start begins collecting metrics.
// It subscribes to events and starts recording metrics.
func (r *MetricsRecorder) Start(ctx context.Context) error {
	r.logger.Debug("Starting metrics recorder")

	// Add test metric
	testGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "test_metric",
		Help: "Test metric to verify prometheus integration",
	})
	testGauge.Set(42.0)

	// Add periodic metric value verification
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Get current metric values
				snapshot := &dto.Metric{}
				r.orderBookMetrics.snapshotsReceived.Write(snapshot)
				r.logger.WithField("value", snapshot.Counter.GetValue()).
					Info("Current snapshot count")

				updates := &dto.Metric{}
				r.orderBookMetrics.updatesReceived.Write(updates)
				r.logger.WithField("value", updates.Counter.GetValue()).
					Info("Current update count")
			}
		}
	}()

	// Subscribe to events and start recording
	go r.recordMetrics(ctx)
	return nil
}

// recordMetrics handles the actual metrics collection
func (r *MetricsRecorder) recordMetrics(ctx context.Context) {
	snapshots := r.eventBus.Subscribe(common.TypeBookSnapshot)

	updates := r.eventBus.Subscribe(common.TypeBookUpdate)

	r.logger.Debug("Subscribed to channels")

	// including for debugging metrics
	snapshotCount := 0
	updateCount := 0

	for {
		select {
		case <-ctx.Done():
			r.logger.Debug("Context cancelled, stopping metrics recorder")
			r.eventBus.Unsubscribe(common.TypeBookSnapshot, snapshots)
			r.eventBus.Unsubscribe(common.TypeBookUpdate, updates)
			return
		case event, ok := <-snapshots:
			if !ok {
				r.logger.Error("Snapshot channel closed unexpectedly")
				return
			}
			snapshotCount++
			// r.logger.WithFields("snapshot_count", snapshotCount).Trace("Received snapshot event")
			r.logger.WithFields(logger.Fields{
				"total_snapshots": snapshotCount,
				"payload_size":    len(event.([]byte)),
			}).Debug("Received snapshot event")
			if msg, ok := event.([]byte); ok {
				r.logger.Trace("Received snapshot event")
				r.recordSnapshot(msg)
			}
		case event, ok := <-updates:
			if !ok {
				r.logger.Error("Update channel closed unexpectedly")
				return
			}
			updateCount++
			r.logger.WithFields(logger.Fields{
				"total_updates": updateCount,
				"payload_size":  len(event.([]byte)),
			}).Debug("Received update event")
			if msg, ok := event.([]byte); ok {
				r.logger.Trace("Received update event")
				r.recordUpdate(msg)
			}
		}
	}
}

// recordSnapshot records metrics for a snapshot message
func (r *MetricsRecorder) recordSnapshot(msg []byte) {
	start := time.Now()

	// Record basic metrics
	r.orderBookMetrics.snapshotsReceived.Inc()
	r.logger.WithField("total_snapshots", r.orderBookMetrics.snapshotsReceived).Debug("Incremented snapshot counter")
	r.orderBookMetrics.snapshotSize.Observe(float64(len(msg)))
	r.wsMetrics.messageReceived.WithLabelValues("snapshot").Inc()

	// Record processing latency
	r.orderBookMetrics.processLatency.Observe(time.Since(start).Seconds())

	// Calculate and record price spread if possible
	if spread, err := r.calculatePriceSpread(msg); err == nil {
		r.orderBookMetrics.priceSpread.Set(spread)
	}
}

// recordUpdate records metrics for an update message
func (r *MetricsRecorder) recordUpdate(msg []byte) {
	start := time.Now()

	// Record basic metrics
	r.orderBookMetrics.updatesReceived.Inc()
	r.logger.WithField("total_updates", r.orderBookMetrics.updatesReceived).Debug("Incremented update counter")
	r.wsMetrics.messageReceived.WithLabelValues("update").Inc()
	r.orderBookMetrics.snapshotSize.Observe(float64(len(msg)))

	// Record processing latency
	latency := time.Since(start).Seconds()
	r.orderBookMetrics.processLatency.Observe(latency)
	r.logger.WithField("latency", latency).Debug("Recorded update latency")

}

// calculatePriceSpread parses a message and calculates the bid-ask spread
func (r *MetricsRecorder) calculatePriceSpread(msg []byte) (float64, error) {
	var data struct {
		Bids [][2]string `json:"bs"`
		Asks [][2]string `json:"as"`
	}

	if err := json.Unmarshal(msg, &data); err != nil {
		return 0, fmt.Errorf("failed to parse price data: %w", err)
	}

	if len(data.Bids) == 0 || len(data.Asks) == 0 {
		return 0, fmt.Errorf("no price data available")
	}

	bestBid, err := strconv.ParseFloat(data.Bids[0][0], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse bid price: %w", err)
	}

	bestAsk, err := strconv.ParseFloat(data.Asks[0][0], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse ask price: %w", err)
	}

	return bestAsk - bestBid, nil
}

func (r *MetricsRecorder) Done() <-chan struct{} {
	return r.done
}
