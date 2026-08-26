//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The schemas in contracts/health-api.yaml, checked against the live services.
// The unit tests assert the same shapes against fakes; this asserts that what
// actually goes over the wire matches.

var allowedReasons = map[string]bool{
	"timeout": true, "connection_refused": true,
	"auth_failed": true, "protocol_error": true, "unknown": true,
}

func TestLivenessResponseMatchesSchema(t *testing.T) {
	for name, base := range map[string]string{"api": apiBase(), "worker": workerBase()} {
		t.Run(name, func(t *testing.T) {
			code, body := get(t, base+"/v1/health/live")
			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200", code)
			}

			var raw map[string]any
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatalf("body is not JSON: %v\n%s", err, body)
			}

			// additionalProperties: false
			required := map[string]bool{"status": true, "service": true, "version": true}
			for field := range raw {
				if !required[field] {
					t.Errorf("unexpected field %q; the schema forbids additional properties", field)
				}
			}
			for field := range required {
				if _, ok := raw[field]; !ok {
					t.Errorf("required field %q is missing", field)
				}
			}
			if raw["status"] != "alive" {
				t.Errorf("status = %v, want the constant \"alive\"", raw["status"])
			}
		})
	}
}

func TestReadinessResponseMatchesSchema(t *testing.T) {
	for name, base := range map[string]string{"api": apiBase(), "worker": workerBase()} {
		t.Run(name, func(t *testing.T) {
			code, body := get(t, base+"/v1/health/ready")
			if code != http.StatusOK && code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 200 or 503", code)
			}

			var raw map[string]any
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatalf("body is not JSON: %v\n%s", err, body)
			}

			allowed := map[string]bool{
				"status": true, "service": true, "version": true,
				"evaluated_at": true, "shutting_down": true, "dependencies": true,
			}
			for field := range raw {
				if !allowed[field] {
					t.Errorf("unexpected field %q; the schema forbids additional properties", field)
				}
			}
			for _, field := range []string{"status", "service", "version", "evaluated_at", "dependencies"} {
				if _, ok := raw[field]; !ok {
					t.Errorf("required field %q is missing", field)
				}
			}

			if s := raw["status"]; s != "ready" && s != "not_ready" {
				t.Errorf("status = %v, want ready or not_ready", s)
			}
			if _, err := time.Parse(time.RFC3339Nano, raw["evaluated_at"].(string)); err != nil {
				t.Errorf("evaluated_at is not a date-time: %v", err)
			}

			deps, ok := raw["dependencies"].([]any)
			if !ok || len(deps) < 3 {
				t.Fatalf("dependencies = %v, want at least 3 entries", raw["dependencies"])
			}

			depAllowed := map[string]bool{"name": true, "healthy": true, "duration_ms": true, "reason": true}
			names := map[string]bool{"kafka": true, "clickhouse": true, "redis": true}
			for _, entry := range deps {
				dep := entry.(map[string]any)
				for field := range dep {
					if !depAllowed[field] {
						t.Errorf("unexpected dependency field %q", field)
					}
				}
				for _, field := range []string{"name", "healthy", "duration_ms"} {
					if _, ok := dep[field]; !ok {
						t.Errorf("dependency entry is missing %q: %v", field, dep)
					}
				}
				if !names[dep["name"].(string)] {
					t.Errorf("dependency name %v is outside the schema enum", dep["name"])
				}
				if d, ok := dep["duration_ms"].(float64); !ok || d < 0 {
					t.Errorf("duration_ms = %v, want a non-negative integer", dep["duration_ms"])
				}
				if reason, present := dep["reason"]; present {
					if dep["healthy"] == true {
						t.Errorf("healthy dependency %v carries a reason", dep["name"])
					}
					if !allowedReasons[reason.(string)] {
						t.Errorf("reason %v is outside the schema enum", reason)
					}
				}
			}
		})
	}
}

func TestMetricsEndpointServesPrometheusText(t *testing.T) {
	for name, base := range map[string]string{"api": apiBase(), "worker": workerBase()} {
		t.Run(name, func(t *testing.T) {
			code, body := get(t, base+"/metrics")
			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200", code)
			}
			for _, want := range []string{
				"pulseflow_build_info",
				"pulseflow_http_requests_total",
				"pulseflow_http_request_duration_seconds",
				"pulseflow_dependency_up",
				"pulseflow_dependency_check_duration_seconds",
			} {
				if !strings.Contains(string(body), want) {
					t.Errorf("metrics output is missing %s", want)
				}
			}
		})
	}
}
