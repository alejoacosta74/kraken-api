package metrics

import (
	"context"
	"fmt"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SystemCollector implements the prometheus.Collector interface to expose Go runtime metrics.
// It collects memory statistics, garbage collection metrics, and concurrency information.
type SystemCollector struct {
	memStats   *prometheus.GaugeVec // Vector of gauges for various memory statistics
	gcStats    *prometheus.GaugeVec // Vector of gauges for garbage collection metrics
	goroutines prometheus.Gauge     // Single gauge for number of goroutines
	threads    prometheus.Gauge     // Single gauge for number of OS threads
	ticker     *time.Ticker         // Ticker to trigger collection
	done       chan struct{}        // Channel to signal the collector has stopped
	logger     *logger.Logger
}

// NewSystemCollector creates and initializes a new SystemCollector with all required Prometheus metrics.
// It sets up gauge vectors for memory and GC stats, and individual gauges for goroutines and threads.
func NewSystemCollector() *SystemCollector {
	collector := &SystemCollector{
		memStats: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "system",
				Name:      "memory_bytes",
				Help:      "Memory statistics in bytes.",
			},
			[]string{"type"}, // Label to differentiate different memory metrics
		),
		gcStats: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "system",
				Name:      "gc_stats",
				Help:      "Garbage collector statistics.",
			},
			[]string{"type"}, // Label to differentiate different GC metrics
		),
		goroutines: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "system",
				Name:      "goroutines",
				Help:      "Number of running goroutines.",
			},
		),
		threads: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "system",
				Name:      "threads",
				Help:      "Number of OS threads created.",
			},
		),
		done:   make(chan struct{}),
		logger: logger.WithField("component", "system_collector"),
	}

	// self register collector with prometheus default registry
	prometheus.MustRegister(collector)

	return collector
}

// Describe implements prometheus.Collector interface.
// It sends all possible metric descriptors to the provided channel,
// and provides the collector with a way to send metric descriptors to the registry.
func (c *SystemCollector) Describe(ch chan<- *prometheus.Desc) {
	c.memStats.Describe(ch)
	c.gcStats.Describe(ch)
	ch <- c.goroutines.Desc()
	ch <- c.threads.Desc()
}

// Collect implements prometheus.Collector interface.
// It is called by Prometheus when collecting metrics. It gathers runtime statistics
// and sends the current values of all metrics to the provided channel.
func (c *SystemCollector) Collect(ch chan<- prometheus.Metric) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m) // Read current memory statistics

	// Memory metrics - tracking various aspects of memory usage
	c.memStats.WithLabelValues("alloc").Set(float64(m.Alloc))            // Currently allocated heap memory
	c.memStats.WithLabelValues("total_alloc").Set(float64(m.TotalAlloc)) // Total bytes allocated (including freed)
	c.memStats.WithLabelValues("sys").Set(float64(m.Sys))                // Total memory obtained from OS
	c.memStats.WithLabelValues("heap_alloc").Set(float64(m.HeapAlloc))   // Memory allocated and still in use
	c.memStats.WithLabelValues("heap_sys").Set(float64(m.HeapSys))       // Memory reserved by heap from OS
	c.memStats.WithLabelValues("heap_idle").Set(float64(m.HeapIdle))     // Memory in idle spans
	c.memStats.WithLabelValues("heap_inuse").Set(float64(m.HeapInuse))   // Memory in currently used spans

	// GC metrics - tracking garbage collector performance
	c.gcStats.WithLabelValues("num_gc").Set(float64(m.NumGC))                // Number of completed GC cycles
	c.gcStats.WithLabelValues("pause_total_ns").Set(float64(m.PauseTotalNs)) // Total time spent in GC pauses

	// Concurrency metrics
	c.goroutines.Set(float64(runtime.NumGoroutine()))            // Current number of goroutines
	c.threads.Set(float64(pprof.Lookup("threadcreate").Count())) // Total OS threads created

	// Send all metrics to the provided channel
	c.memStats.Collect(ch)
	c.gcStats.Collect(ch)
	c.goroutines.Collect(ch)
	c.threads.Collect(ch)
}

type systemStatsContainer struct {
	m              runtime.MemStats
	numGC          uint32
	pauseTotalNs   uint64
	goroutineCount int
	threadCount    int
}

func (s *systemStatsContainer) update() {
	runtime.ReadMemStats(&s.m)
	s.numGC = s.m.NumGC
	s.pauseTotalNs = s.m.PauseTotalNs
	s.goroutineCount = runtime.NumGoroutine()
	s.threadCount = pprof.Lookup("threadcreate").Count()
}

// Start starts the collection process.
func (c *SystemCollector) Start(ctx context.Context) error {
	c.ticker = time.NewTicker(10 * time.Second)

	go func() {
		var stats systemStatsContainer
		for {
			select {
			case <-ctx.Done():
				c.ticker.Stop()
				close(c.done)
				return
			case <-c.ticker.C:
				// update all system stats
				stats.update()
				// record memory metrics
				c.memStats.WithLabelValues("alloc").Set(float64(stats.m.Alloc))
				c.memStats.WithLabelValues("total_alloc").Set(float64(stats.m.TotalAlloc))
				c.memStats.WithLabelValues("sys").Set(float64(stats.m.Sys))
				c.memStats.WithLabelValues("heap_alloc").Set(float64(stats.m.HeapAlloc))
				c.memStats.WithLabelValues("heap_sys").Set(float64(stats.m.HeapSys))
				c.memStats.WithLabelValues("heap_idle").Set(float64(stats.m.HeapIdle))
				c.memStats.WithLabelValues("heap_inuse").Set(float64(stats.m.HeapInuse))

				// record gc stats
				c.gcStats.WithLabelValues("num_gc").Set(float64(stats.numGC))
				c.gcStats.WithLabelValues("pause_total_ns").Set(float64(stats.pauseTotalNs))

				// record concurrency stats
				c.goroutines.Set(float64(stats.goroutineCount))
				c.threads.Set(float64(stats.threadCount))

				// log stats
				c.logStats(stats)
			}
		}
	}()

	return nil
}

func (c *SystemCollector) Done() <-chan struct{} {
	return c.done
}

func (c *SystemCollector) logStats(stats systemStatsContainer) {
	s := fmt.Sprintf(`
	Application Stats:
	  Memory:
		Alloc: %v MB          // Currently allocated heap memory
		TotalAlloc: %v MB     // Cumulative bytes allocated (including freed)
		Sys: %v MB           // Total memory obtained from OS
		HeapAlloc: %v MB     // Memory allocated and still in use
		HeapSys: %v MB       // Memory reserved by heap from OS
		HeapIdle: %v MB      // Memory in idle spans (returned to OS)
		HeapInuse: %v MB     // Memory in currently used spans
	  Garbage Collector:
		NumGC: %v            // Number of completed GC cycles
		PauseTotalNs: %v ms  // Total time spent in GC pauses
		LastGC: %v seconds ago // Time since last garbage collection
	
	  Concurrency:
		Goroutines: %v       // Current number of running goroutines
		Threads: %v          // Total OS threads created
	`,
		bToMb(stats.m.Alloc),
		bToMb(stats.m.TotalAlloc),
		bToMb(stats.m.Sys),
		bToMb(stats.m.HeapAlloc),
		bToMb(stats.m.HeapSys),
		bToMb(stats.m.HeapIdle),
		bToMb(stats.m.HeapInuse),
		stats.numGC,
		stats.pauseTotalNs/1e6,
		time.Since(time.Unix(0, int64(stats.m.LastGC))).Seconds(),
		stats.goroutineCount,
		stats.threadCount,
	)

	c.logger.Info(s)

}

// bToMb converts bytes to megabytes
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
