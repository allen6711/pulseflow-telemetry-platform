//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"
)

// spawnedOutput accumulates a spawned process's output for later assertions.
type spawnedOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *spawnedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *spawnedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

// lastSpawned holds the output of the most recently spawned process. Tests in
// this package run sequentially, so a single slot is sufficient.
var lastSpawned *spawnedOutput

// readSpawnedLogs returns everything the most recently spawned process wrote.
func readSpawnedLogs(t *testing.T) string {
	t.Helper()
	if lastSpawned == nil {
		t.Fatal("no process has been spawned in this test")
	}
	return lastSpawned.String()
}

// spawnAPI starts a locally built api binary on its own port with the given
// extra environment, and stops it when the test ends.
//
// It runs the binary directly rather than through compose because the point is
// to control what the process sees at the moment it starts.
func spawnAPI(t *testing.T, port int, env ...string) string {
	t.Helper()

	bin := repoRoot(t) + "/bin/api"
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("bin/api is not built; run `make build` first: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = repoRoot(t)

	// Capture output so tests can assert on what was logged.
	var out spawnedOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	lastSpawned = &out
	cmd.Env = append(os.Environ(),
		"PULSEFLOW_HTTP_PORT="+strconv.Itoa(port),
		// Short windows so the test observes transitions rather than waiting
		// out the production defaults.
		"PULSEFLOW_HEALTH_CHECK_TIMEOUT=2s",
		"PULSEFLOW_HEALTH_CACHE_TTL=500ms",
	)
	cmd.Env = append(cmd.Env, env...)

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("starting api: %v", err)
	}

	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	base := "http://localhost:" + strconv.Itoa(port)

	// The process must come up even with dependencies unreachable, so we wait
	// on liveness, which by contract never consults one.
	if err := eventually(t, 15*time.Second, func() error {
		resp, err := http.Get(base + "/v1/health/live")
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("liveness returned %d", resp.StatusCode)
		}
		return nil
	}); err != nil {
		t.Fatalf("the service never started: %v", err)
	}

	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		t.Fatal("the service exited instead of starting with dependencies unavailable")
	}

	return base
}

// FR-037: with every dependency unreachable, the service still starts, reports
// not-ready, and does not exit. Getting this wrong is the difference between a
// stack that comes up on the first try and one that CrashLoopBackOffs.
func TestServiceStartsWithAllDependenciesUnreachable(t *testing.T) {
	// Ports nothing is listening on.
	base := spawnAPI(t, 18190,
		"PULSEFLOW_KAFKA_BROKERS=127.0.0.1:19199",
		"PULSEFLOW_CLICKHOUSE_ADDR=127.0.0.1:19198",
		"PULSEFLOW_REDIS_ADDR=127.0.0.1:19197",
	)

	code, got := fetchReadiness(t, base)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("readiness = %d, want 503 with every dependency unreachable", code)
	}
	if got.Status != "not_ready" {
		t.Errorf("status = %q, want not_ready", got.Status)
	}
	if len(got.Dependencies) != len(dependencyNames) {
		t.Errorf("dependencies = %d, want all %d listed", len(got.Dependencies), len(dependencyNames))
	}
	for _, dep := range got.Dependencies {
		if dep.Healthy {
			t.Errorf("%s reported healthy against a closed port", dep.Name)
		}
	}

	// Liveness is unaffected: the process is alive, just not ready.
	if code, _ := get(t, base+"/v1/health/live"); code != http.StatusOK {
		t.Errorf("liveness = %d, want 200", code)
	}
}

// The second half of FR-037: a process that has never successfully connected
// must still become ready on its own once the dependency appears, with no
// operator intervention.
func TestServiceBecomesReadyOnceDependenciesAppear(t *testing.T) {
	stopService(t, "redis")

	base := spawnAPI(t, 18191,
		"PULSEFLOW_KAFKA_BROKERS="+envOr("PULSEFLOW_TEST_KAFKA", "localhost:9092"),
		"PULSEFLOW_CLICKHOUSE_ADDR="+envOr("PULSEFLOW_TEST_CLICKHOUSE", "localhost:9000"),
		"PULSEFLOW_REDIS_ADDR="+envOr("PULSEFLOW_TEST_REDIS", "localhost:6379"),
	)

	if code, _ := fetchReadiness(t, base); code != http.StatusServiceUnavailable {
		t.Fatalf("readiness = %d, want 503 while redis is stopped", code)
	}

	if out, err := compose(t, "start", "redis"); err != nil {
		t.Fatalf("starting redis: %v\n%s", err, out)
	}

	if err := eventually(t, 30*time.Second, func() error {
		code, _ := fetchReadiness(t, base)
		if code == http.StatusOK {
			return nil
		}
		return errReadiness("readiness is still " + strconv.Itoa(code))
	}); err != nil {
		t.Errorf("a service that never connected did not become ready once redis appeared: %v", err)
	}
}
