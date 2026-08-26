package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestAggregator(t *testing.T, ttl time.Duration, checkers ...Checker) *Aggregator {
	t.Helper()
	return NewAggregator(AggregatorOptions{
		Service:  "pulseflow-api",
		Version:  "v1",
		Checkers: checkers,
		Timeout:  time.Second,
		CacheTTL: ttl,
	})
}

// byName indexes a result's dependencies for assertions.
func byName(r Result) map[string]Status {
	out := make(map[string]Status, len(r.Dependencies))
	for _, d := range r.Dependencies {
		out[d.Name] = d
	}
	return out
}

func TestAllHealthyReportsReady(t *testing.T) {
	agg := newTestAggregator(t, 0,
		&stubChecker{name: DependencyKafka},
		&stubChecker{name: DependencyClickHouse},
		&stubChecker{name: DependencyRedis},
	)

	got := agg.Evaluate(context.Background())

	if got.Status != StatusReady {
		t.Errorf("status = %q, want %q", got.Status, StatusReady)
	}
	if len(got.Dependencies) != 3 {
		t.Fatalf("dependencies = %d, want 3", len(got.Dependencies))
	}
	for _, d := range got.Dependencies {
		if !d.Healthy {
			t.Errorf("%s reported unhealthy", d.Name)
		}
		if d.Reason != "" {
			t.Errorf("%s carries a reason while healthy: %q", d.Name, d.Reason)
		}
	}
}

// FR-008: the response lists every dependency, not just the failing one and not
// just the first failure encountered.
func TestSingleFailureStillListsEveryDependency(t *testing.T) {
	agg := newTestAggregator(t, 0,
		&stubChecker{name: DependencyKafka},
		&stubChecker{name: DependencyClickHouse, err: errors.New("clickhouse ping: unexpected packet")},
		&stubChecker{name: DependencyRedis},
	)

	got := agg.Evaluate(context.Background())

	if got.Status != StatusNotReady {
		t.Errorf("status = %q, want %q", got.Status, StatusNotReady)
	}
	if len(got.Dependencies) != 3 {
		t.Fatalf("dependencies = %d, want all 3 listed", len(got.Dependencies))
	}

	deps := byName(got)
	if deps[DependencyClickHouse].Healthy {
		t.Error("clickhouse should be unhealthy")
	}
	if !deps[DependencyKafka].Healthy || !deps[DependencyRedis].Healthy {
		t.Error("healthy dependencies must still be reported as healthy")
	}
}

func TestAllFailuresAreReported(t *testing.T) {
	agg := newTestAggregator(t, 0,
		&stubChecker{name: DependencyKafka, err: errors.New("kafka metadata request failed")},
		&stubChecker{name: DependencyClickHouse, err: context.DeadlineExceeded},
		&stubChecker{name: DependencyRedis, err: errors.New("redis ping: bad protocol")},
	)

	got := agg.Evaluate(context.Background())

	if got.Status != StatusNotReady {
		t.Errorf("status = %q, want %q", got.Status, StatusNotReady)
	}
	for _, d := range got.Dependencies {
		if d.Healthy {
			t.Errorf("%s reported healthy while every checker failed", d.Name)
		}
		if d.Reason == "" {
			t.Errorf("%s is unhealthy but carries no reason", d.Name)
		}
	}
}

// Checks run concurrently, so worst-case latency is one timeout rather than the
// sum of all three.
func TestChecksRunConcurrently(t *testing.T) {
	const delay = 200 * time.Millisecond
	agg := newTestAggregator(t, 0,
		&stubChecker{name: DependencyKafka, delay: delay},
		&stubChecker{name: DependencyClickHouse, delay: delay},
		&stubChecker{name: DependencyRedis, delay: delay},
	)

	start := time.Now()
	agg.Evaluate(context.Background())
	elapsed := time.Since(start)

	if elapsed >= 3*delay {
		t.Errorf("evaluation took %s; three %s checks appear to have run sequentially", elapsed, delay)
	}
}

func TestRecoveryFlipsBackToReady(t *testing.T) {
	failing := &stubChecker{name: DependencyClickHouse, err: errors.New("clickhouse ping: connection reset")}
	agg := newTestAggregator(t, 0,
		&stubChecker{name: DependencyKafka},
		failing,
		&stubChecker{name: DependencyRedis},
	)

	if got := agg.Evaluate(context.Background()); got.Status != StatusNotReady {
		t.Fatalf("status = %q, want %q while the dependency is down", got.Status, StatusNotReady)
	}

	failing.err = nil

	if got := agg.Evaluate(context.Background()); got.Status != StatusReady {
		t.Errorf("status = %q, want %q after the dependency recovered", got.Status, StatusReady)
	}
}
