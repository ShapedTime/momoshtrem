package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server serves Prometheus metrics on a dedicated HTTP port.
type Server struct {
	httpServer *http.Server
	log        *slog.Logger
}

// NewServer creates a metrics server exposing /metrics on the given port.
func NewServer(port int, reg *prometheus.Registry) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	return &Server{
		httpServer: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		log: slog.With("component", "metrics-server"),
	}
}

// Start begins serving metrics. Blocks until the server stops.
func (s *Server) Start() error {
	s.log.Info("starting metrics server", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.log.Error("metrics server error", "error", err)
		return err
	}
	return nil
}

// Shutdown gracefully stops the metrics server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
