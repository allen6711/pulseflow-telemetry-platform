//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"
)

// stopService stops one compose service and restores it when the test ends.
func stopService(t *testing.T, service string) {
	t.Helper()

	if out, err := compose(t, "stop", service); err != nil {
		t.Fatalf("stopping %s: %v\n%s", service, err, out)
	}

	t.Cleanup(func() {
		if out, err := compose(t, "start", service); err != nil {
			t.Fatalf("restoring %s: %v\n%s", service, err, out)
		}
		// Leave the stack ready for whatever runs next.
		_ = eventually(t, 60*time.Second, func() error {
			if code, _ := fetchReadiness(t, apiBase()); code != http.StatusOK {
				return errNotReady
			}
			return nil
		})
	})
}

// errNotReady keeps the retry helper allocation-free in the hot path.
var errNotReady = errReadiness("service is not ready")

type errReadiness string

func (e errReadiness) Error() string { return string(e) }

// FR-008: 503 naming the failing dependency, with the healthy ones still listed.
func TestStoppedDependencyMakesReadinessFailAndNamesIt(t *testing.T) {
	cases := []struct {
		service    string
		dependency string
	}{
		{"clickhouse", "clickhouse"},
		{"redis", "redis"},
		{"kafka", "kafka"},
	}

	for _, tc := range cases {
		t.Run(tc.service, func(t *testing.T) {
			stopService(t, tc.service)

			var got readiness
			err := eventually(t, 30*time.Second, func() error {
				code, r := fetchReadiness(t, apiBase())
				got = r
				if code != http.StatusServiceUnavailable {
					return errReadiness("readiness has not reported 503 yet")
				}
				return nil
			})
			if err != nil {
				t.Fatalf("with %s stopped: %v\nlast response: %+v", tc.service, err, got)
			}

			if got.Status != "not_ready" {
				t.Errorf("status = %q, want not_ready", got.Status)
			}

			if len(got.Dependencies) != len(dependencyNames) {
				t.Errorf("dependencies = %d, want all %d listed", len(got.Dependencies), len(dependencyNames))
			}

			var failing, healthy int
			for _, dep := range got.Dependencies {
				if dep.Name == tc.dependency {
					if dep.Healthy {
						t.Errorf("%s reported healthy while stopped", dep.Name)
					}
					if dep.Reason == "" {
						t.Errorf("%s is unhealthy but carries no reason", dep.Name)
					}
					failing++
					continue
				}
				if dep.Healthy {
					healthy++
				}
			}
			if failing != 1 {
				t.Errorf("the stopped dependency %q was not reported as failing", tc.dependency)
			}
			if healthy != len(dependencyNames)-1 {
				t.Errorf("healthy dependencies = %d, want %d still reported healthy", healthy, len(dependencyNames)-1)
			}
		})
	}
}

// FR-012: the response is externally reachable, so a failure reason must be a
// classification and never raw driver text carrying hosts or credentials.
func TestFailureReasonStaysWithinTheBoundedVocabulary(t *testing.T) {
	stopService(t, "redis")

	allowed := map[string]bool{
		"timeout": true, "connection_refused": true,
		"auth_failed": true, "protocol_error": true, "unknown": true,
	}

	var got readiness
	err := eventually(t, 30*time.Second, func() error {
		code, r := fetchReadiness(t, apiBase())
		got = r
		if code != http.StatusServiceUnavailable {
			return errReadiness("not failing yet")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("%v", err)
	}

	for _, dep := range got.Dependencies {
		if dep.Healthy || dep.Reason == "" {
			continue
		}
		if !allowed[dep.Reason] {
			t.Errorf("%s reason %q is outside the bounded vocabulary", dep.Name, dep.Reason)
		}
	}
}
