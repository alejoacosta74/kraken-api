package system

import (
	"log"
	"os"
	"runtime"
	"runtime/pprof"

	_ "net/http/pprof"
)

// StartProfiling initiates CPU profiling for performance analysis.
//
// This function enables CPU profiling by creating a profile file at the specified path
// and starting the profiler. CPU profiling is crucial for identifying performance
// bottlenecks and understanding where the program spends most of its execution time.
//
// Parameters:
//   - cpuprofile: Path where the CPU profile file should be created. If empty, no profiling occurs.
//   - memprofile: Currently unused, reserved for future memory profiling integration.
//
// The function will:
// 1. Create a new file at cpuprofile path
// 2. Start CPU profiling and write data to this file
// 3. Defer stopping the profiler and closing the file
//
// Note: This should typically be called at program startup, with StopProfiling called
// before program termination. The resulting profile can be analyzed using:
//   - go tool pprof [binary] [profile_file]
//   - or via the web interface at http://localhost:6060/debug/pprof
func StartProfiling(cpuprofile, memprofile string) {
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal("could not create cpu profile file")
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start cpu profile")
		}
		defer pprof.StopCPUProfile()
	}
}

// StopProfiling writes a memory profile to disk for performance analysis.
//
// This function captures a snapshot of the program's memory usage by writing a heap profile
// to the specified file path. Memory profiling is essential for:
// - Identifying memory leaks
// - Understanding memory allocation patterns
// - Analyzing heap usage and garbage collection behavior
// - Optimizing memory-intensive operations
//
// Parameters:
//   - memprofile: Path where the memory profile file should be created. If empty, no profiling occurs.
//
// The function will:
// 1. Create a new file at the memprofile path
// 2. Force a garbage collection to get accurate memory statistics
// 3. Write the heap profile data to the file
//
// The resulting profile can be analyzed using:
//   - go tool pprof [binary] [profile_file]
//   - or via the web interface at http://localhost:6060/debug/pprof/heap
//
// Note: This should typically be called before program termination to capture the final
// memory state. For continuous memory profiling, consider using the runtime/pprof HTTP
// interface instead.
func StopProfiling(memprofile string) {
	if memprofile != "" {
		f, err := os.Create(memprofile)
		if err != nil {
			log.Fatal("could not create memory profile file")
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile")
		}
	}
}
