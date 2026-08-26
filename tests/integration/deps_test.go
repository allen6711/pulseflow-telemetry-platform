//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// readiness is the response shape from contracts/health-api.yaml.
type readiness struct {
	Status       string `json:"status"`
	Service      string `json:"service"`
	Version      string `json:"version"`
	EvaluatedAt  string `json:"evaluated_at"`
	ShuttingDown bool   `json:"shutting_down"`
	Dependencies []struct {
		Name       string `json:"name"`
		Healthy    bool   `json:"healthy"`
		DurationMS int64  `json:"duration_ms"`
		Reason     string `json:"reason"`
	} `json:"dependencies"`
}

// dependencyNames are the three dependencies every readiness response must list.
var dependencyNames = []string{"kafka", "clickhouse", "redis"}

func fetchReadiness(t *testing.T, base string) (int, readiness) {
	t.Helper()
	code, body := get(t, base+"/v1/health/ready")

	var got readiness
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("readiness body is not valid JSON: %v\nbody: %s", err, body)
	}
	return code, got
}

func TestReadinessReportsEveryDependencyAgainstTheLiveStack(t *testing.T) {
	code, got := fetchReadiness(t, apiBase())

	if code != http.StatusOK {
		t.Fatalf("readiness status = %d, want 200 against a healthy stack. Body: %+v", code, got)
	}
	if got.Status != "ready" {
		t.Errorf("status = %q, want ready", got.Status)
	}

	seen := map[string]bool{}
	for _, dep := range got.Dependencies {
		seen[dep.Name] = true
		if !dep.Healthy {
			t.Errorf("%s is unhealthy against a live stack: reason=%q", dep.Name, dep.Reason)
		}
		if dep.Reason != "" {
			t.Errorf("%s carries reason %q while healthy", dep.Name, dep.Reason)
		}
	}
	for _, name := range dependencyNames {
		if !seen[name] {
			t.Errorf("dependency %q is missing from the response", name)
		}
	}
}

func TestBothServicesProbeTheSameDependencies(t *testing.T) {
	for name, base := range map[string]string{"api": apiBase(), "worker": workerBase()} {
		t.Run(name, func(t *testing.T) {
			code, got := fetchReadiness(t, base)
			if code != http.StatusOK {
				t.Fatalf("readiness status = %d, want 200", code)
			}
			if len(got.Dependencies) != len(dependencyNames) {
				t.Errorf("dependencies = %d, want %d", len(got.Dependencies), len(dependencyNames))
			}
		})
	}
}

func TestDependencyMetricsAreExposed(t *testing.T) {
	// A readiness probe first, so the gauges have been set at least once.
	fetchReadiness(t, apiBase())

	_, body := get(t, apiBase()+"/metrics")
	metrics := string(body)

	for _, name := range dependencyNames {
		want := `pulseflow_dependency_up{dependency="` + name + `"}`
		if !strings.Contains(metrics, want) {
			t.Errorf("metrics are missing %s", want)
		}
	}
}
