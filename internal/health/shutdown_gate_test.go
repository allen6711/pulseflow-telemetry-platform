package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// FR-020: readiness reports not-ready the moment shutdown begins, before
// anything starts closing. Flipping it later would race the load balancer and
// drop requests that were already in flight.
func TestReadinessFlipsBeforeAnythingElseCloses(t *testing.T) {
	agg := newTestAggregator(t, 0,
		&stubChecker{name: DependencyKafka},
		&stubChecker{name: DependencyClickHouse},
		&stubChecker{name: DependencyRedis},
	)

	rec := httptest.NewRecorder()
	ReadinessHandler(agg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ReadyPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 before shutdown", rec.Code)
	}

	agg.BeginShutdown()

	rec = httptest.NewRecorder()
	ReadinessHandler(agg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ReadyPath, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 immediately after shutdown begins", rec.Code)
	}
}

// The gate overrides dependency state entirely: healthy dependencies do not
// make a draining process ready again.
func TestShutdownOverridesHealthyDependencies(t *testing.T) {
	agg := newTestAggregator(t, 0, &stubChecker{name: DependencyKafka})

	// Populate the cache with a healthy result first.
	if got := agg.Evaluate(context.Background()); got.Status != StatusReady {
		t.Fatalf("status = %q, want ready", got.Status)
	}

	agg.BeginShutdown()

	got := agg.Evaluate(context.Background())
	if got.Status != StatusNotReady {
		t.Errorf("status = %q, want not_ready despite healthy dependencies", got.Status)
	}
	if !got.ShuttingDown {
		t.Error("shutting_down must be reported")
	}
	if len(got.Dependencies) == 0 {
		t.Error("the dependency list must still be present while draining")
	}
}

// The transition is one-way: nothing brings a draining process back.
func TestShutdownIsIrreversible(t *testing.T) {
	failing := &stubChecker{name: DependencyRedis, err: errors.New("redis ping: connection reset")}
	agg := newTestAggregator(t, 0, failing)

	agg.BeginShutdown()
	failing.err = nil

	for range 3 {
		if got := agg.Evaluate(context.Background()); got.Status != StatusNotReady {
			t.Fatalf("status = %q, want not_ready; shutdown must not be reversible", got.Status)
		}
	}
}
