package stats

import (
	"context"
	"fmt"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/alejoacosta74/go-logger"
)

type SystemStats struct {
	logger *logger.Logger
	done   chan struct{}
}

// func getHeapProfile() string {
// 	if heapProf := pprof.Lookup("heap"); heapProf != nil {
// 		var b bytes.Buffer
// 		heapProf.WriteTo(&b, 1)
// 		return b.String()
// 	}
// 	return ""
// }

func NewSystemStats() *SystemStats {
	return &SystemStats{
		logger: logger.WithField("component", "system_stats"),
		done:   make(chan struct{}),
	}
}

func (s *SystemStats) Start(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	s.logStats() // Log initial stats

	for {
		select {
		case <-ctx.Done():
			close(s.done)
			return nil
		case <-ticker.C:
			s.logStats()
		}
	}
}

func (s *SystemStats) logStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	goroutineCount := runtime.NumGoroutine()
	threadCount := pprof.Lookup("threadcreate").Count()

	stats := fmt.Sprintf(`
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
		bToMb(m.Alloc),
		bToMb(m.TotalAlloc),
		bToMb(m.Sys),
		bToMb(m.HeapAlloc),
		bToMb(m.HeapSys),
		bToMb(m.HeapIdle),
		bToMb(m.HeapInuse),
		m.NumGC,
		m.PauseTotalNs/1e6,
		time.Since(time.Unix(0, int64(m.LastGC))).Seconds(),
		goroutineCount,
		threadCount,
	)

	s.logger.Info(stats)
}

func (s *SystemStats) Done() <-chan struct{} {
	return s.done
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
