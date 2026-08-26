//go:build integration

// Package integration exercises PulseFlow against the real local stack.
//
// These tests assume `make up` has already run. They deliberately do not start
// the stack themselves: the artifact under test is the compose file a developer
// actually uses, so testing anything else would test the wrong thing.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Supporting services declared by docker-compose.yml.
var supportingServices = []string{"kafka", "clickhouse", "redis", "prometheus"}

// Application services.
var appServices = []string{"api", "worker"}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func apiBase() string    { return envOr("PULSEFLOW_TEST_API_BASE", "http://localhost:8080") }
func workerBase() string { return envOr("PULSEFLOW_TEST_WORKER_BASE", "http://localhost:8081") }

// compose runs a docker compose subcommand from the repository root.
func compose(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// repoRoot resolves the repository root from this package's location.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolving working directory: %v", err)
	}
	// tests/integration -> repository root
	return wd + "/../.."
}

// serviceHealth returns the compose health state of one service.
func serviceHealth(t *testing.T, service string) string {
	t.Helper()
	out, err := compose(t, "ps", "--format", "json", service)
	if err != nil {
		t.Fatalf("docker compose ps %s: %v\n%s", service, err, out)
	}

	// `compose ps --format json` emits one JSON object per line.
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var row struct {
			Service string `json:"Service"`
			State   string `json:"State"`
			Health  string `json:"Health"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		if row.Service != service {
			continue
		}
		if row.Health != "" {
			return row.Health
		}
		return row.State
	}
	return ""
}

// get performs a GET and returns the status code and body.
func get(t *testing.T, url string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building request for %s: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body of %s: %v", url, err)
	}
	return resp.StatusCode, body
}

// eventually retries fn until it returns nil or the deadline passes.
func eventually(t *testing.T, timeout time.Duration, fn func() error) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last error
	for {
		if last = fn(); last == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("condition not met within %s: %w", timeout, last)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func TestAllSupportingServicesReportHealthy(t *testing.T) {
	for _, svc := range supportingServices {
		t.Run(svc, func(t *testing.T) {
			got := serviceHealth(t, svc)
			if got != "healthy" {
				t.Errorf("%s health = %q, want healthy. Run `make up` first.", svc, got)
			}
		})
	}
}

func TestApplicationServicesAreUp(t *testing.T) {
	for _, svc := range appServices {
		t.Run(svc, func(t *testing.T) {
			got := serviceHealth(t, svc)
			if got != "healthy" && got != "running" {
				t.Errorf("%s state = %q, want healthy or running", svc, got)
			}
		})
	}
}

func TestBothServicesServeLivenessAndMetrics(t *testing.T) {
	for name, base := range map[string]string{"api": apiBase(), "worker": workerBase()} {
		t.Run(name, func(t *testing.T) {
			code, body := get(t, base+"/v1/health/live")
			if code != http.StatusOK {
				t.Errorf("liveness status = %d, want 200", code)
			}
			var live struct {
				Status  string `json:"status"`
				Service string `json:"service"`
				Version string `json:"version"`
			}
			if err := json.Unmarshal(body, &live); err != nil {
				t.Fatalf("liveness body is not JSON: %v\nbody: %s", err, body)
			}
			if live.Status != "alive" {
				t.Errorf("liveness status field = %q, want alive", live.Status)
			}

			code, metrics := get(t, base+"/metrics")
			if code != http.StatusOK {
				t.Errorf("metrics status = %d, want 200", code)
			}
			if !strings.Contains(string(metrics), "pulseflow_build_info") {
				t.Error("metrics output is missing pulseflow_build_info")
			}
		})
	}
}

func TestPrometheusScrapesBothServices(t *testing.T) {
	prom := envOr("PULSEFLOW_TEST_PROMETHEUS_BASE", "http://localhost:9090")

	err := eventually(t, 30*time.Second, func() error {
		code, body := get(t, prom+"/api/v1/targets?state=active")
		if code != http.StatusOK {
			return fmt.Errorf("targets endpoint returned %d", code)
		}
		var payload struct {
			Data struct {
				ActiveTargets []struct {
					Health string            `json:"health"`
					Labels map[string]string `json:"labels"`
				} `json:"activeTargets"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("parsing targets: %w", err)
		}

		up := map[string]bool{}
		for _, target := range payload.Data.ActiveTargets {
			if target.Health == "up" {
				up[target.Labels["job"]] = true
			}
		}
		for _, job := range []string{"pulseflow-api", "pulseflow-worker"} {
			if !up[job] {
				return fmt.Errorf("job %s is not up", job)
			}
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
