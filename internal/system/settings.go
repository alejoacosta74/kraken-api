package system

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/alejoacosta74/go-logger"
	"github.com/spf13/viper"
)

// Settings holds system-wide configuration
type Settings struct {
	MaxProcs     int
	GCPercent    int
	MaxThreads   int
	MaxStackSize int // in bytes
	MemoryLimit  int // in MB
	logger       *logger.Logger
}

// DefaultSettings returns recommended default settings
func DefaultSettings() *Settings {
	return &Settings{
		MaxProcs:     runtime.NumCPU(),
		GCPercent:    100,         // Default GC target percentage
		MaxThreads:   10000,       // Default max OS threads
		MaxStackSize: 1024 * 1024, // 1MB stack size
		MemoryLimit:  1024 * 2,    // 2GB memory limit
		logger:       logger.WithField("component", "system_settings"),
	}
}

// Apply configures system-wide settings
func (s *Settings) Apply() error {
	s.logger.Info("Applying system settings...")

	// Set maximum number of CPUs to use
	runtime.GOMAXPROCS(s.MaxProcs)
	s.logger.Infof("GOMAXPROCS set to %d", s.MaxProcs)

	// Configure garbage collection
	debug.SetGCPercent(s.GCPercent)
	s.logger.Infof("GC percent set to %d", s.GCPercent)

	debug.SetMaxThreads(s.MaxThreads)
	s.logger.Infof("Max threads set to %d", s.MaxThreads)

	// Set maximum stack size per goroutine
	debug.SetMaxStack(s.MaxStackSize)
	s.logger.Infof("Max stack size set to %d bytes", s.MaxStackSize)

	// Set memory limit
	if err := s.setMemoryLimit(); err != nil {
		return fmt.Errorf("failed to set memory limit: %w", err)
	}
	s.logger.Infof("Memory limit set to %dMB", s.MemoryLimit)

	return nil
}

// setMemoryLimit configures memory limits using GOGC and other runtime settings
func (s *Settings) setMemoryLimit() error {
	// Convert MB to bytes
	memoryLimitBytes := int64(s.MemoryLimit) * 1024 * 1024

	// Set soft memory limit using GOGC
	debug.SetMemoryLimit(memoryLimitBytes)

	return nil
}

// WithMaxProcs sets the maximum number of CPUs to use
func (s *Settings) WithMaxProcs(n int) *Settings {
	s.MaxProcs = n
	return s
}

// WithGCPercent sets the GC target percentage
func (s *Settings) WithGCPercent(percent int) *Settings {
	s.GCPercent = percent
	return s
}

// WithMaxThreads sets the maximum number of OS threads
func (s *Settings) WithMaxThreads(n int) *Settings {
	s.MaxThreads = n
	return s
}

// WithMemoryLimit sets the memory limit in MB
func (s *Settings) WithMemoryLimit(mb int) *Settings {
	s.MemoryLimit = mb
	return s
}

// LoadFromViper loads settings from Viper configuration
func LoadFromViper() *Settings {
	return DefaultSettings().
		WithMaxProcs(viper.GetInt("system.maxprocs")).
		WithGCPercent(viper.GetInt("system.gcpercent")).
		WithMaxThreads(viper.GetInt("system.maxthreads")).
		WithMemoryLimit(viper.GetInt("system.memorylimit"))
}
