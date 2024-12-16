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

func getServerAddr(s *MetricsServer) (string, error) {
	// Create a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to connect to the server until it's ready or timeout
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for server to start")
		default:
			// Get the listener from the server
			// if s.server.Addr == "" {
			// 	time.Sleep(10 * time.Millisecond)
			// 	continue
			// }

			// Try to connect to see if it's ready
			conn, err := net.DialTimeout("tcp", "localhost:0", 100*time.Millisecond)
			if err == nil {
				conn.Close()
				return "http://" + s.server.Addr, nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestMetricsServer(t *testing.T) {
	// Reset Prometheus registry for tests
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	tests := []struct {
		name         string
		endpoint     string
		expectedCode int
		setupMetrics func()
		validateBody func(*testing.T, []byte)
	}{
		{
			name:         "health endpoint returns 200",
			endpoint:     "/health",
			expectedCode: http.StatusOK,
			setupMetrics: func() {},
			validateBody: func(t *testing.T, body []byte) {},
		},
		{
			name:         "metrics endpoint returns prometheus metrics",
			endpoint:     "/metrics",
			expectedCode: http.StatusOK,
			setupMetrics: func() {
				// Add a test metric
				testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
					Name: "test_metric",
					Help: "Test metric",
				})
				registry.MustRegister(testGauge)
				testGauge.Set(42.0)
			},
			validateBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "test_metric 42")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test metrics
			tt.setupMetrics()

			port, err := getFreePort()
			require.NoError(t, err)

			// Create server with random port
			server := NewMetricsServer(fmt.Sprintf("localhost:%d", port))

			// Start server in goroutine
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				time.Sleep(3 * time.Second)
				cancel()
			}()

			errCh := make(chan error, 1)
			go func() {
				err := server.Start(ctx)
				if err != nil {
					fmt.Println("Error starting server:", err)
				}
				require.NoError(t, err)
				errCh <- err
			}()

			serverAddr, err := getServerAddr(server)
			require.NoError(t, err)

			// Make request
			resp, err := http.Get(serverAddr + tt.endpoint)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Check status code
			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			// Read and validate body
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			tt.validateBody(t, body)

			// Test graceful shutdown
			cancel()
			select {
			case <-server.done:
				// Server shut down successfully
			case <-time.After(6 * time.Second):
				t.Fatal("Server didn't shut down within timeout")
			case err := <-errCh:
				require.NoError(t, err)
			}
		})
	}
}
