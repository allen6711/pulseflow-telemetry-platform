package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// decodeReadiness parses a readiness response into a generic map so the test
// asserts on the wire shape from contracts/health-api.yaml rather than on the
// Go struct, which would pass even if the JSON tags were wrong.
func decodeReadiness(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, body)
	}
	return got
}

func serveReadiness(t *testing.T, agg *Aggregator) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	ReadinessHandler(agg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ReadyPath, nil))
	return rec, decodeReadiness(t, rec.Body.Bytes())
}

func TestReadyResponseMatchesContract(t *testing.T) {
	agg := newTestAggregator(t, 0,
		&stubChecker{name: DependencyKafka},
		&stubChecker{name: DependencyClickHouse},
		&stubChecker{name: DependencyRedis},
	)

	rec, got := serveReadiness(t, agg)

	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	for _, field := range []string{"status", "service", "version", "evaluated_at", "dependencies"} {
		if _, ok := got[field]; !ok {
			t.Errorf("required field %q is missing", field)
		}
	}
	if got["status"] != StatusReady {
		t.Errorf("status = %v, want %q", got["status"], StatusReady)
	}
	// shutting_down is omitted rather than false when not shutting down.
	if _, present := got["shutting_down"]; present {
		t.Error("shutting_down should be omitted while the service is running")
	}
	if _, err := time.Parse(time.RFC3339Nano, got["evaluated_at"].(string)); err != nil {
		t.Errorf("evaluated_at is not RFC3339: %v", err)
	}

	deps, ok := got["dependencies"].([]any)
	if !ok || len(deps) != 3 {
		t.Fatalf("dependencies = %v, want 3 entries", got["dependencies"])
	}
	for _, raw := range deps {
		dep := raw.(map[string]any)
		for _, field := range []string{"name", "healthy", "duration_ms"} {
			if _, ok := dep[field]; !ok {
				t.Errorf("dependency entry is missing %q: %v", field, dep)
			}
		}
		if _, present := dep["reason"]; present {
			t.Errorf("reason should be omitted for a healthy dependency: %v", dep)
		}
	}
}

func TestUnhealthyDependencyMapsTo503AndNamesTheFailure(t *testing.T) {
	agg := newTestAggregator(t, 0,
		&stubChecker{name: DependencyKafka},
		&stubChecker{name: DependencyClickHouse, err: errors.New("clickhouse ping: unexpected packet")},
		&stubChecker{name: DependencyRedis},
	)

	rec, got := serveReadiness(t, agg)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want 503", rec.Code)
	}
	if got["status"] != StatusNotReady {
		t.Errorf("status = %v, want %q", got["status"], StatusNotReady)
	}

	deps := got["dependencies"].([]any)
	if len(deps) != 3 {
		t.Fatalf("dependencies = %d, want all 3 listed even on failure", len(deps))
	}

	var found bool
	for _, raw := range deps {
		dep := raw.(map[string]any)
		if dep["name"] != DependencyClickHouse {
			continue
		}
		found = true
		if dep["healthy"] != false {
			t.Error("clickhouse should be reported unhealthy")
		}
		reason, ok := dep["reason"].(string)
		if !ok || reason == "" {
			t.Error("an unhealthy dependency must carry a reason")
		}
		switch Reason(reason) {
		case ReasonTimeout, ReasonConnectionRefused, ReasonAuthFailed, ReasonProtocolError, ReasonUnknown:
		default:
			t.Errorf("reason %q is outside the bounded vocabulary", reason)
		}
	}
	if !found {
		t.Error("the failing dependency was not present in the response")
	}
}

func TestShutdownMapsTo503WithShuttingDownFlag(t *testing.T) {
	agg := newTestAggregator(t, 0, &stubChecker{name: DependencyKafka})
	agg.BeginShutdown()

	rec, got := serveReadiness(t, agg)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want 503 while shutting down", rec.Code)
	}
	if got["shutting_down"] != true {
		t.Errorf("shutting_down = %v, want true", got["shutting_down"])
	}
}

// The response is externally reachable, so it must not carry connection
// strings, credentials, or raw driver text (FR-012).
func TestResponseDoesNotLeakSensitiveDetail(t *testing.T) {
	agg := newTestAggregator(t, 0,
		&stubChecker{
			name: DependencyRedis,
			err:  errors.New("redis ping: dial tcp 10.1.2.3:6379: WRONGPASS hunter2"),
		},
	)

	rec, _ := serveReadiness(t, agg)

	body := rec.Body.String()
	for _, secret := range []string{"hunter2", "10.1.2.3", "6379", "dial tcp"} {
		if strings.Contains(body, secret) {
			t.Errorf("response leaked %q:\n%s", secret, body)
		}
	}
}
