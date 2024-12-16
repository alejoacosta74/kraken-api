package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsServer handles exposing metrics via HTTP
type MetricsServer struct {
	server *http.Server
	logger *logger.Logger
	done   chan struct{}
}

func NewMetricsServer(addr string) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return &MetricsServer{
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
		logger: logger.WithField("component", "metrics_server"),
		done:   make(chan struct{}),
	}
}

func (s *MetricsServer) Start(ctx context.Context) error {
	// Add logging middleware
	loggingHandler := promhttp.InstrumentMetricHandler(
		prometheus.DefaultRegisterer,
		promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
			EnableOpenMetrics: true,
			Registry:          prometheus.DefaultRegisterer,
		}),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		s.logger.Debugf("Metrics request from %s", r.RemoteAddr)
		loggingHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		s.logger.Debug("Shutting down metrics server")
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Error shutting down server:", err)
		}
		close(s.done)
	}()

	if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
		s.logger.Error("Error starting server:", err)
		return err
	}
	s.logger.Info("Metrics server shutdown complete")
	return nil
}
