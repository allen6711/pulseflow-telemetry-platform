// Package httpserver wires PulseFlow's HTTP surface: route registration, the
// observability middleware chain, and ordered shutdown.
package httpserver

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/allen6711/pulseflow-telemetry-platform/internal/observability"
)

// MetricsPath is the Prometheus scrape endpoint, exposed by both binaries.
const MetricsPath = "/metrics"

// Options configures a Server.
type Options struct {
	Port     int
	Logger   *slog.Logger
	Registry *prometheus.Registry
	Metrics  *observability.HTTPMetrics
}

// Server is an HTTP server with the project's middleware chain already applied.
type Server struct {
	mux  *http.ServeMux
	srv  *http.Server
	log  *slog.Logger
	addr string
}

// New returns a Server with /metrics already registered.
func New(opts Options) *Server {
	mux := http.NewServeMux()
	addr := net.JoinHostPort("", strconv.Itoa(opts.Port))

	s := &Server{
		mux:  mux,
		log:  opts.Logger,
		addr: addr,
		srv: &http.Server{
			Addr:              addr,
			Handler:           observe(mux, opts.Logger, opts.Metrics),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	s.Handle(MetricsPath, promhttp.HandlerFor(opts.Registry, promhttp.HandlerOpts{
		Registry: opts.Registry,
	}))

	return s
}

// Handle registers a handler for a ServeMux pattern. The pattern becomes the
// route label on this endpoint's metrics, so it must stay a fixed string.
func (s *Server) Handle(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

// Addr returns the address the server listens on.
func (s *Server) Addr() string { return s.addr }

// ListenAndServe blocks until the server stops. It returns nil on a clean
// shutdown, so callers do not have to special-case http.ErrServerClosed.
func (s *Server) ListenAndServe() error {
	err := s.srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown stops accepting new connections and waits for in-flight requests,
// bounded by ctx.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
