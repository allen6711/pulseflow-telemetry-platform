//go:build integration

package integration

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// canaries are values planted in the configuration so a leak is unambiguous:
// if one of these strings appears in a log line or an HTTP response, a secret
// escaped (FR-012, FR-018, FR-030).
var canaries = []string{
	"canary-clickhouse-pw-8f14e45f",
	"canary-redis-pw-c9f0f895fb",
}

// FR-030: no sensitive value reaches the log output, including inside an error.
func TestSecretsNeverReachTheLogs(t *testing.T) {
	base := spawnAPIWithSecrets(t, 18290)

	// Exercise the paths that touch credentials: readiness authenticates
	// against ClickHouse and Redis, and failures attach driver errors.
	for range 3 {
		get(t, base+"/v1/health/ready")
		time.Sleep(200 * time.Millisecond)
	}

	logs := readSpawnedLogs(t)
	for _, canary := range canaries {
		if strings.Contains(logs, canary) {
			t.Errorf("a secret reached the log output: %q\n%s", canary, excerpt(logs, canary))
		}
	}
	if !strings.Contains(logs, "***") {
		t.Error("the startup configuration summary did not mask anything; is it being emitted?")
	}
}

// FR-012: the readiness response is externally reachable and must not carry
// connection strings, credentials, or raw driver text.
func TestSecretsNeverReachTheReadinessResponse(t *testing.T) {
	base := spawnAPIWithSecrets(t, 18291)

	_, body := get(t, base+"/v1/health/ready")
	response := string(body)

	for _, canary := range canaries {
		if strings.Contains(response, canary) {
			t.Errorf("a secret reached the readiness response: %q\n%s", canary, response)
		}
	}
	// Raw driver text would carry these; a classified reason does not.
	for _, marker := range []string{"dial tcp", "127.0.0.1", "password"} {
		if strings.Contains(response, marker) {
			t.Errorf("raw driver detail %q reached the response:\n%s", marker, response)
		}
	}
}

func TestSecretsNeverReachTheMetricsEndpoint(t *testing.T) {
	base := spawnAPIWithSecrets(t, 18292)

	get(t, base+"/v1/health/ready")
	_, body := get(t, base+"/metrics")

	for _, canary := range canaries {
		if strings.Contains(string(body), canary) {
			t.Errorf("a secret reached the metrics output: %q", canary)
		}
	}
}

// spawnAPIWithSecrets starts an api process configured with canary credentials
// and unreachable dependencies, so both the success and failure log paths run.
func spawnAPIWithSecrets(t *testing.T, port int) string {
	t.Helper()
	return spawnAPI(t, port,
		"PULSEFLOW_CLICKHOUSE_PASSWORD="+canaries[0],
		"PULSEFLOW_REDIS_PASSWORD="+canaries[1],
		"PULSEFLOW_KAFKA_BROKERS=127.0.0.1:"+strconv.Itoa(port+100),
		"PULSEFLOW_CLICKHOUSE_ADDR=127.0.0.1:"+strconv.Itoa(port+101),
		"PULSEFLOW_REDIS_ADDR=127.0.0.1:"+strconv.Itoa(port+102),
	)
}

// excerpt returns the log line containing needle, for a readable failure.
func excerpt(logs, needle string) string {
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
