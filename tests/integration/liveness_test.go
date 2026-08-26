//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// SC-003: with any or all dependencies unavailable, liveness keeps succeeding,
// with zero false negatives.
//
// This is the check that prevents a dependency outage from restarting every
// replica. A liveness probe that quietly consults a dependency looks fine
// locally and turns into CrashLoopBackOff the moment F11 puts these services
// behind an orchestrator.
func TestLivenessSurvivesEveryDependencyFailureCombination(t *testing.T) {
	combinations := [][]string{
		{"clickhouse"},
		{"redis"},
		{"kafka"},
		{"kafka", "clickhouse", "redis"},
	}

	for _, stopped := range combinations {
		t.Run(label(stopped), func(t *testing.T) {
			for _, svc := range stopped {
				stopService(t, svc)
			}

			// Give readiness time to notice, so we are genuinely probing
			// liveness while the process knows it is degraded.
			_ = eventually(t, 30*time.Second, func() error {
				if code, _ := fetchReadiness(t, apiBase()); code == http.StatusServiceUnavailable {
					return nil
				}
				return errReadiness("readiness has not degraded yet")
			})

			for name, base := range map[string]string{"api": apiBase(), "worker": workerBase()} {
				// Probe repeatedly: one lucky success would hide an
				// intermittent dependency coupling.
				for attempt := range 5 {
					code, body := get(t, base+"/v1/health/live")
					if code != http.StatusOK {
						t.Fatalf("%s liveness returned %d on attempt %d with %v stopped; body: %s",
							name, code, attempt+1, stopped, body)
					}

					var live struct {
						Status string `json:"status"`
					}
					if err := json.Unmarshal(body, &live); err != nil {
						t.Fatalf("%s liveness body is not JSON: %v", name, err)
					}
					if live.Status != "alive" {
						t.Errorf("%s liveness status = %q, want alive", name, live.Status)
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		})
	}
}

func label(services []string) string {
	if len(services) == 1 {
		return services[0] + " stopped"
	}
	return "all stopped"
}
