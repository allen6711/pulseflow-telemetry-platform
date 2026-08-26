package health

import (
	"context"
	"sync"
	"testing"
	"time"
)

// FR-010: a readiness probe every second across N replicas must not become N
// dependency queries per second. Without collapsing and caching, F09's
// benchmark would be measuring its own probe traffic alongside the real load.
func TestConcurrentEvaluationsCollapseIntoOneCheckRound(t *testing.T) {
	kafka := &stubChecker{name: DependencyKafka, delay: 50 * time.Millisecond}
	agg := newTestAggregator(t, time.Second, kafka)

	const callers = 50
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			agg.Evaluate(context.Background())
		}()
	}
	wg.Wait()

	if rounds := agg.CheckRounds(); rounds != 1 {
		t.Errorf("%d concurrent evaluations triggered %d check rounds, want 1", callers, rounds)
	}
	if kafka.calls != 1 {
		t.Errorf("the dependency was probed %d times, want 1", kafka.calls)
	}
}

func TestSequentialEvaluationsInsideTTLReuseTheCachedResult(t *testing.T) {
	kafka := &stubChecker{name: DependencyKafka}
	agg := newTestAggregator(t, time.Hour, kafka)

	for range 20 {
		agg.Evaluate(context.Background())
	}

	if rounds := agg.CheckRounds(); rounds != 1 {
		t.Errorf("20 evaluations inside one TTL window triggered %d rounds, want 1", rounds)
	}
}

func TestEvaluationAfterTTLExpiryProbesAgain(t *testing.T) {
	kafka := &stubChecker{name: DependencyKafka}
	agg := newTestAggregator(t, 20*time.Millisecond, kafka)

	agg.Evaluate(context.Background())
	time.Sleep(40 * time.Millisecond)
	agg.Evaluate(context.Background())

	if rounds := agg.CheckRounds(); rounds != 2 {
		t.Errorf("check rounds = %d, want 2 once the TTL expired", rounds)
	}
}

// Shutdown short-circuits everything: not-ready is reported immediately,
// without probing and without waiting for the cache to expire (FR-020).
func TestShutdownReportsNotReadyWithoutProbing(t *testing.T) {
	kafka := &stubChecker{name: DependencyKafka}
	agg := newTestAggregator(t, 0, kafka)

	agg.BeginShutdown()
	got := agg.Evaluate(context.Background())

	if got.Status != StatusNotReady {
		t.Errorf("status = %q, want %q", got.Status, StatusNotReady)
	}
	if !got.ShuttingDown {
		t.Error("shutting_down must be true once shutdown has begun")
	}
	if agg.CheckRounds() != 0 {
		t.Errorf("dependencies were probed %d times during shutdown, want 0", agg.CheckRounds())
	}
	if len(got.Dependencies) != 1 {
		t.Errorf("dependencies = %d, want the full list even during shutdown", len(got.Dependencies))
	}
}
