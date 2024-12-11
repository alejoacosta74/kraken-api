// Package metrics provides functionality for collecting and exposing system metrics
// using Prometheus. It tracks various aspects of the system including order book
// operations, WebSocket performance, and Kafka message handling.
package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/events"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsCollector handles collecting and exposing metrics about the system.
// It subscribes to system events and maintains various Prometheus metrics
// to track system performance and behavior.
type MetricsCollector struct {
	// orderBookMetrics contains all metrics related to order book operations
	orderBookMetrics struct {
		// Counter: Increments each time a snapshot is received
		snapshotsReceived prometheus.Counter

		// Counter: Increments each time an update is received
		updatesReceived prometheus.Counter

		// Histogram: Tracks the time taken to process messages
		processLatency prometheus.Histogram

		// Histogram: Tracks the size of snapshots in bytes
		snapshotSize prometheus.Histogram

		// Gauge: Current spread between best bid and ask
		priceSpread prometheus.Gauge
	}

	// wsMetrics contains all metrics related to WebSocket operations
	wsMetrics struct {
		// CounterVec: Tracks messages received by type (snapshot, update, etc.)
		messageReceived *prometheus.CounterVec

		// Counter: Number of connection errors
		connectionErrors prometheus.Counter

		// Counter: Number of reconnection attempts
		reconnectCount prometheus.Counter

		// Histogram: Time taken to process WebSocket messages
		messageLatency prometheus.Histogram
	}

	// kafkaMetrics contains all metrics related to Kafka operations
	kafkaMetrics struct {
		// CounterVec: Messages sent to Kafka by topic
		messagesSent *prometheus.CounterVec

		// Counter: Number of Kafka send errors
		sendErrors prometheus.Counter

		// Histogram: Time taken to send messages to Kafka
		messageLatency prometheus.Histogram

		// Histogram: Size of message batches sent to Kafka
		batchSize prometheus.Histogram

		// Gauge: Current size of the producer worker pool
		producerPoolSize prometheus.Gauge
	}

	// Event bus for receiving system events
	eventBus events.Bus

	// Logger for metric-related logging
	logger *logger.Logger
}

// Config holds the configuration for the metrics collector.
// It allows customization of the metrics collection and exposure.
type Config struct {
	// MetricsAddr is the address where the Prometheus metrics will be exposed
	// Example: ":2112" will expose metrics on http://localhost:2112/metrics
	MetricsAddr string
}

// NewMetricsCollector creates and registers all Prometheus metrics.
// It initializes all metric types (counters, gauges, histograms) with appropriate
// labels and configurations.
//
// Parameters:
//   - eventBus: Event bus to subscribe to for metric events
//   - cfg: Configuration for the metrics collector
//
// Returns:
//   - A fully configured MetricsCollector ready to start collecting metrics
func NewMetricsCollector(eventBus events.Bus, cfg Config) *MetricsCollector {
	m := &MetricsCollector{
		eventBus: eventBus,
		logger:   logger.Log,
	}

	// Initialize Order Book metrics
	m.orderBookMetrics.snapshotsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orderbook",
		Name:      "snapshots_received_total",
		Help:      "Total number of order book snapshots received",
	})

	m.orderBookMetrics.updatesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "orderbook",
		Name:      "updates_received_total",
		Help:      "Total number of order book updates received",
	})

	m.orderBookMetrics.processLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "orderbook",
		Name:      "process_latency_seconds",
		Help:      "Time taken to process order book messages",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
	})

	m.orderBookMetrics.snapshotSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "orderbook",
		Name:      "snapshot_size_bytes",
		Help:      "Size of order book snapshots in bytes",
		Buckets:   prometheus.ExponentialBuckets(1000, 2, 10), // Start at 1KB, double 10 times
	})

	m.orderBookMetrics.priceSpread = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "orderbook",
		Name:      "price_spread",
		Help:      "Current spread between best bid and ask prices",
	})

	// Initialize WebSocket metrics
	m.wsMetrics.messageReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "websocket",
			Name:      "messages_received_total",
			Help:      "Total number of WebSocket messages received by type",
		},
		[]string{"type"},
	)

	m.wsMetrics.connectionErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "websocket",
		Name:      "connection_errors_total",
		Help:      "Total number of WebSocket connection errors",
	})

	m.wsMetrics.messageLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "websocket",
		Name:      "message_latency_seconds",
		Help:      "Latency of WebSocket message processing",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
	})

	// Initialize Kafka metrics
	m.kafkaMetrics.messagesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "kafka",
			Name:      "messages_sent_total",
			Help:      "Total number of messages sent to Kafka by topic",
		},
		[]string{"topic"},
	)

	m.kafkaMetrics.sendErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "kafka",
		Name:      "send_errors_total",
		Help:      "Total number of Kafka send errors",
	})

	m.kafkaMetrics.messageLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "kafka",
		Name:      "message_latency_seconds",
		Help:      "Latency of Kafka message production",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
	})

	return m
}

// Start begins collecting metrics and serving them via HTTP.
// It performs the following tasks:
// 1. Starts the HTTP server for Prometheus scraping
// 2. Subscribes to relevant system events
// 3. Begins collecting and updating metrics
//
// Parameters:
//   - ctx: Context for cancellation
//   - cfg: Configuration for the metrics collector
//
// Returns:
//   - error: Any error that occurred during startup
func (m *MetricsCollector) Start(ctx context.Context, cfg Config) error {
	// Start the metrics HTTP server in a separate goroutine
	go m.serveMetrics(cfg.MetricsAddr)

	// Subscribe to events and start collecting metrics
	m.subscribeToEvents()

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// serveMetrics starts the Prometheus metrics HTTP server.
// It sets up the following endpoints:
// - /metrics: Prometheus metrics endpoint
// - /health: Basic health check endpoint
//
// Parameters:
//   - addr: Address to listen on (e.g., ":2112")
func (m *MetricsCollector) serveMetrics(addr string) {
	// Create a new mux for metrics endpoints
	mux := http.NewServeMux()

	// Register the Prometheus metrics handler
	// This endpoint will be scraped by Prometheus
	mux.Handle("/metrics", promhttp.Handler())

	// Add a basic health check endpoint
	// This can be used by monitoring systems to check if the service is alive
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})

	// Configure and start the HTTP server
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start serving metrics
	// Log any errors that aren't due to normal server shutdown
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		m.logger.Error("Metrics server error:", err)
	}
}

// subscribeToEvents sets up all event subscriptions for metric collection.
// It subscribes to various system events and updates metrics accordingly.
// Each event type is handled in its own goroutine to prevent blocking.
func (m *MetricsCollector) subscribeToEvents() {
	// Subscribe to order book events
	snapshots := m.eventBus.Subscribe("book_snapshot")
	updates := m.eventBus.Subscribe("book_update")

	// Handle order book snapshots
	go func() {
		for event := range snapshots {
			if msg, ok := event.([]byte); ok {
				// Start timing the processing
				start := time.Now()

				// Update various metrics for snapshot processing
				m.orderBookMetrics.snapshotsReceived.Inc()
				m.orderBookMetrics.snapshotSize.Observe(float64(len(msg)))
				m.wsMetrics.messageReceived.WithLabelValues("snapshot").Inc()

				// Record processing latency
				m.orderBookMetrics.processLatency.Observe(time.Since(start).Seconds())
			}
		}
	}()

	// Handle order book updates
	go func() {
		for event := range updates {
			if msg, ok := event.([]byte); ok {
				// Start timing the processing
				start := time.Now()

				// Update metrics for update messages
				m.orderBookMetrics.updatesReceived.Inc()
				m.wsMetrics.messageReceived.WithLabelValues("update").Inc()

				// Record message size and latency
				m.orderBookMetrics.snapshotSize.Observe(float64(len(msg)))
				m.orderBookMetrics.processLatency.Observe(time.Since(start).Seconds())

				// Optionally, we could parse the message to get the price spread
				if spread, err := m.calculatePriceSpread(msg); err == nil {
					m.orderBookMetrics.priceSpread.Set(spread)
				}
			}
		}
	}()
}

// calculatePriceSpread parses an update message and calculates the bid-ask spread
func (m *MetricsCollector) calculatePriceSpread(msg []byte) (float64, error) {
	var update struct {
		Data []struct {
			Bids [][2]string `json:"bs"`
			Asks [][2]string `json:"as"`
		} `json:"data"`
	}

	if err := json.Unmarshal(msg, &update); err != nil {
		return 0, err
	}

	// Ensure we have both bids and asks
	if len(update.Data) == 0 ||
		(len(update.Data[0].Bids) == 0 && len(update.Data[0].Asks) == 0) {
		return 0, fmt.Errorf("no price data in update")
	}

	// Parse best bid and ask prices
	var bestBid, bestAsk float64
	if len(update.Data[0].Bids) > 0 {
		bestBid, _ = strconv.ParseFloat(update.Data[0].Bids[0][0], 64)
	}
	if len(update.Data[0].Asks) > 0 {
		bestAsk, _ = strconv.ParseFloat(update.Data[0].Asks[0][0], 64)
	}

	// Calculate spread
	return bestAsk - bestBid, nil
}

// RecordKafkaMessage records metrics about Kafka message production.
// This method should be called by Kafka producers after attempting to send a message.
//
// Parameters:
//   - topic: The Kafka topic the message was sent to
//   - latency: How long it took to send the message
//   - err: Any error that occurred during sending
func (m *MetricsCollector) RecordKafkaMessage(topic string, latency time.Duration, err error) {
	if err != nil {
		// Record failed message sends
		m.kafkaMetrics.sendErrors.Inc()
	} else {
		// Record successful message sends and their latency
		m.kafkaMetrics.messagesSent.WithLabelValues(topic).Inc()
		m.kafkaMetrics.messageLatency.Observe(latency.Seconds())
	}
}
