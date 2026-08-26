package httpserver

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/allen6711/pulseflow-telemetry-platform/internal/logging"
	"github.com/allen6711/pulseflow-telemetry-platform/internal/observability"
)

// routeUnmatched is the route label used for requests that match no registered
// pattern. Using a constant rather than the request path keeps an unbounded
// input from becoming an unbounded label.
const routeUnmatched = "unmatched"

// statusRecorder captures the status code so it can be reported as a metric
// label and a log field.
type statusRecorder struct {
	http.ResponseWriter
	code    int
	written bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.written {
		r.code = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.code = http.StatusOK
		r.written = true
	}
	return r.ResponseWriter.Write(b)
}

// observe wraps the mux with correlation, logging, metrics, and panic recovery.
//
// The route label is resolved through mux.Handler before serving, because a
// ServeMux sets Request.Pattern on its own copy of the request and the outer
// middleware would never see it.
func observe(mux *http.ServeMux, log *slog.Logger, metrics *observability.HTTPMetrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		_, pattern := mux.Handler(r)
		route := pattern
		if route == "" {
			route = routeUnmatched
		}

		// Adopt the caller's trace ID when it sent one, so a request's logs
		// correlate across process boundaries (FR-026, FR-027).
		traceID := logging.TraceIDForRequest(r.Header.Get(logging.TraceparentHeader))
		ctx := logging.ContextWithTraceID(r.Context(), traceID)
		r = r.WithContext(ctx)

		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}

		defer func() {
			if v := recover(); v != nil {
				if !rec.written {
					rec.WriteHeader(http.StatusInternalServerError)
				}
				log.ErrorContext(ctx, "http_request_failed",
					slog.String("route", route),
					slog.String("method", r.Method),
					slog.Int("code", rec.code),
					slog.String("error_class", "panic"),
					slog.String("error", logging.SanitizeString(fmt.Sprint(v))),
				)
			}

			elapsed := time.Since(start)
			metrics.Observe(route, r.Method, rec.code, elapsed)

			log.DebugContext(ctx, "http_request",
				slog.String("route", route),
				slog.String("method", r.Method),
				slog.Int("code", rec.code),
				slog.Int64("duration_ms", elapsed.Milliseconds()),
			)
		}()

		mux.ServeHTTP(rec, r)
	})
}
