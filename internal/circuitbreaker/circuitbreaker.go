package circuitbreaker

import (
	"fmt"
	"sync"
	"time"

	"github.com/alejoacosta74/go-logger"
)

// State represents the current state of the circuit breaker
type State int

const (
	StateClosed   State = iota // Normal operation, requests allowed
	StateOpen                  // Circuit is tripped, requests blocked
	StateHalfOpen              // Testing if service has recovered
)

// CircuitBreaker implements the circuit breaker pattern to prevent cascading failures
// by temporarily stopping operations when a threshold of failures is reached.
type CircuitBreaker struct {
	state     State         // Current state of the circuit breaker
	failures  int           // Count of consecutive failures
	threshold int           // Number of failures before opening circuit
	timeout   time.Duration // How long to wait before attempting recovery
	lastError error         // Most recent error that occurred
	mu        sync.RWMutex  // Protects concurrent access to state
	openTime  time.Time     // When the circuit was opened
	logger    *logger.Logger
}

// NewCircuitBreaker creates a new circuit breaker with the specified failure threshold
// and recovery timeout duration.
//
// Parameters:
//   - threshold: Number of consecutive failures before opening the circuit
//   - timeout: Duration to wait before attempting recovery in half-open state
//
// Returns:
//   - *CircuitBreaker: A new circuit breaker instance in the closed state
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		timeout:   timeout,
		logger:    logger.WithField("component", "circuitbreaker"),
	}
}

// Execute runs the provided function if the circuit breaker allows it.
// Records the result and updates the circuit breaker state accordingly.
//
// Parameters:
//   - fn: The function to execute if circuit is closed
//
// Returns:
//   - error: Error from function execution or circuit breaker open error
func (cb *CircuitBreaker) Execute(fn func() error) error {
	// Check if request is allowed
	if !cb.AllowRequest() {
		return fmt.Errorf("circuit breaker is open: %v", cb.lastError)
	}

	// Execute the function and record the result
	err := fn()
	cb.RecordResult(err)
	return err
}

// AllowRequest checks if a request should be allowed through based on the
// current state of the circuit breaker.
//
// Returns:
//   - bool: true if request should be allowed, false if it should be blocked
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state == StateOpen {
		// Check if timeout has elapsed to transition to half-open
		if time.Since(cb.openTime) > cb.timeout {
			// Need to upgrade lock to modify state
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = StateHalfOpen
			cb.mu.Unlock()
			cb.mu.RLock()
			cb.logger.Warn("Circuit breaker transitioned to half-open")
			return true
		}
		return false
	}
	return true
}

// RecordResult records the result of a request and updates the circuit breaker state.
// Failed requests increment the failure counter and may open the circuit.
// Successful requests reset the failure counter and close the circuit.
//
// Parameters:
//   - err: The error result to record (nil for success)
func (cb *CircuitBreaker) RecordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		// Record failure and check threshold
		cb.failures++
		cb.lastError = err
		if cb.failures >= cb.threshold {
			cb.state = StateOpen
			cb.openTime = time.Now()
			cb.logger.Warn("Circuit breaker opened")
		}
	} else {
		// Reset on success
		cb.failures = 0
		cb.state = StateClosed
		cb.logger.Debug("Circuit breaker closed")
	}
}

func (cb *CircuitBreaker) LastError() error {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.lastError
}
