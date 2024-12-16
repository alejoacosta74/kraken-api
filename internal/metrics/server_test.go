package metrics

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsServer_Start(t *testing.T) {
	// Reset Prometheus registry for tests
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	tests := []struct {
		name           string
		setupMetrics   func(registry *prometheus.Registry)
		validateServer func(*testing.T, string)
		expectedError  error
	}{
		{
			name: "server starts and serves metrics successfully",
			setupMetrics: func(registry *prometheus.Registry) {
				// Register a test metric with the specific registry
				testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
					Name: "test_metric",
					Help: "Test metric",
				})
				registry.MustRegister(testGauge)
				testGauge.Set(42.0)
			},
			validateServer: func(t *testing.T, addr string) {
				// Test metrics endpoint
				resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr))
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusOK, resp.StatusCode)

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), "test_metric 42")

				// Test health endpoint
				resp, err = http.Get(fmt.Sprintf("http://%s/health", addr))
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusOK, resp.StatusCode)
			},
			expectedError: nil,
		},
		{
			name: "server handles concurrent requests",
			setupMetrics: func(registry *prometheus.Registry) {
				counter := prometheus.NewCounter(prometheus.CounterOpts{
					Name: "request_counter",
					Help: "Test counter",
				})
				registry.MustRegister(counter)
			},
			validateServer: func(t *testing.T, addr string) {
				// Make concurrent requests
				for i := 0; i < 10; i++ {
					go func() {
						resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr))
						require.NoError(t, err)
						defer resp.Body.Close()
						assert.Equal(t, http.StatusOK, resp.StatusCode)
					}()
				}
				// Wait for concurrent requests to complete
				time.Sleep(100 * time.Millisecond)
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test metrics with the registry
			tt.setupMetrics(registry)

			// Create a TCP listener first to get an available port
			listener, err := net.Listen("tcp", "localhost:0")
			require.NoError(t, err)

			addr := listener.Addr().String()
			listener.Close()

			// Create server with the known available port
			server := NewMetricsServer(addr)

			// Start server in goroutine
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			errCh := make(chan error, 1)
			go func() {
				errCh <- server.Start(ctx)
			}()

			// Wait for server to start using the helper function
			err = waitForServer(addr, 2*time.Second)
			require.NoError(t, err, "Server failed to start")

			// Run validation
			tt.validateServer(t, addr)

			// Test graceful shutdown
			cancel()

			select {
			case err := <-errCh:
				assert.Equal(t, tt.expectedError, err)
			case <-time.After(5 * time.Second):
				t.Fatal("Server didn't shut down within timeout")
			}

			// Verify server is actually stopped
			_, err = http.Get(fmt.Sprintf("http://%s/metrics", addr))
			assert.Error(t, err, "Server should be stopped")
		})
	}
}

// waitForServer attempts to connect to the server until it's ready or times out
func waitForServer(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := http.Get(fmt.Sprintf("http://%s/health", addr))
		if err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("server failed to start within %v", timeout)
}
