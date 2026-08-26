package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allen6711/pulseflow-telemetry-platform/internal/health"
	"github.com/allen6711/pulseflow-telemetry-platform/internal/observability"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	reg := observability.NewRegistry(observability.BuildInfo{Version: "v1", Commit: "abc123"})
	return New(Options{
		Port:     0,
		Logger:   slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Registry: reg,
		Metrics:  observability.NewHTTPMetrics(reg),
	})
}

// serve routes one request through the full middleware chain.
func serve(s *Server, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestLivenessResponseShape(t *testing.T) {
	s := newTestServer(t)
	s.Handle(health.LivePath, health.LivenessHandler("pulseflow-api", "v1"))

	rec := serve(s, http.MethodGet, health.LivePath)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got health.LivenessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid JSON: %v\nbody: %s", err, rec.Body.String())
	}
	want := health.LivenessResponse{Status: health.StatusAlive, Service: "pulseflow-api", Version: "v1"}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}

func TestMetricsRouteRegisteredByDefault(t *testing.T) {
	s := newTestServer(t)

	rec := serve(s, http.MethodGet, MetricsPath)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"pulseflow_build_info", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output is missing %q", want)
		}
	}
}

func TestHTTPMetricsUseRegisteredPatternAsRouteLabel(t *testing.T) {
	s := newTestServer(t)
	s.Handle(health.LivePath, health.LivenessHandler("pulseflow-api", "v1"))

	serve(s, http.MethodGet, health.LivePath)
	body := serve(s, http.MethodGet, MetricsPath).Body.String()

	want := `pulseflow_http_requests_total{code="200",method="GET",route="` + health.LivePath + `"}`
	if !strings.Contains(body, want) {
		t.Errorf("expected a counter labelled with the registered pattern.\nwant substring: %s", want)
	}
}

func TestUnmatchedRouteDoesNotLeakPathIntoLabels(t *testing.T) {
	s := newTestServer(t)

	// An unbounded input: if this reached the label, cardinality would be
	// unbounded too, which Constitution Principle I forbids.
	serve(s, http.MethodGet, "/v1/does-not-exist/8f14e45fceea167a")
	body := serve(s, http.MethodGet, MetricsPath).Body.String()

	if strings.Contains(body, "8f14e45fceea167a") {
		t.Error("the request path leaked into a metric label")
	}
	if !strings.Contains(body, `route="`+routeUnmatched+`"`) {
		t.Errorf("unmatched request was not labelled %q", routeUnmatched)
	}
}
