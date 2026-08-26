//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"
)

// recoveryBudget is the window SC-004 allows for readiness to return to success
// after a dependency comes back.
const recoveryBudget = 10 * time.Second

// SC-004: after a dependency recovers, readiness returns to success on its own
// within 10 seconds, with no process restart.
//
// The failure this guards against is a latched readiness result: a cache that
// never re-probes, or a client that does not reconnect, leaves the service
// permanently not-ready after a blip that has long since passed.
func TestReadinessRecoversWithoutRestart(t *testing.T) {
	for _, service := range []string{"clickhouse", "redis", "kafka"} {
		t.Run(service, func(t *testing.T) {
			before := containerStartedAt(t, "api")

			if out, err := compose(t, "stop", service); err != nil {
				t.Fatalf("stopping %s: %v\n%s", service, err, out)
			}

			if err := eventually(t, 30*time.Second, func() error {
				if code, _ := fetchReadiness(t, apiBase()); code == http.StatusServiceUnavailable {
					return nil
				}
				return errReadiness("readiness has not degraded yet")
			}); err != nil {
				t.Fatalf("readiness never reported the %s outage: %v", service, err)
			}

			if out, err := compose(t, "start", service); err != nil {
				t.Fatalf("restarting %s: %v\n%s", service, err, out)
			}

			// The clock starts once the dependency is accepting connections
			// again; waiting for compose to report healthy first would measure
			// the container's start-up, not our recovery.
			if err := eventually(t, 60*time.Second, func() error {
				if serviceHealth(t, service) == "healthy" {
					return nil
				}
				return errReadiness(service + " has not become healthy yet")
			}); err != nil {
				t.Fatalf("%s did not come back: %v", service, err)
			}

			start := time.Now()
			if err := eventually(t, recoveryBudget, func() error {
				if code, _ := fetchReadiness(t, apiBase()); code == http.StatusOK {
					return nil
				}
				return errReadiness("readiness has not recovered yet")
			}); err != nil {
				t.Fatalf("readiness did not recover within %s after %s returned: %v",
					recoveryBudget, service, err)
			}
			t.Logf("readiness recovered %s after %s became healthy", time.Since(start).Round(time.Millisecond), service)

			if after := containerStartedAt(t, "api"); after != before {
				t.Errorf("the api container restarted during recovery (%q -> %q); "+
					"readiness must recover without a restart", before, after)
			}
		})
	}
}

// containerStartedAt returns the container's start timestamp, which changes if
// and only if the container restarted.
func containerStartedAt(t *testing.T, service string) string {
	t.Helper()
	out, err := compose(t, "ps", "--format", "{{.Service}} {{.RunningFor}}", service)
	if err != nil {
		t.Fatalf("inspecting %s: %v\n%s", service, err, out)
	}
	return out
}
