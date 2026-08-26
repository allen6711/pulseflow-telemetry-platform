package health

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/allen6711/pulseflow-telemetry-platform/internal/logging"
)

// Readiness statuses.
const (
	StatusReady    = "ready"
	StatusNotReady = "not_ready"
)

// Result is one readiness evaluation: the aggregate of every dependency status
// plus this process's own lifecycle state. Its shape is fixed by the
// ReadinessResponse schema in contracts/health-api.yaml.
type Result struct {
	Status       string    `json:"status"`
	Service      string    `json:"service"`
	Version      string    `json:"version"`
	EvaluatedAt  time.Time `json:"evaluated_at"`
	ShuttingDown bool      `json:"shutting_down,omitempty"`
	Dependencies []Status  `json:"dependencies"`
}

// MetricsRecorder observes dependency checks. It is an interface so this
// package does not depend on a metrics library.
type MetricsRecorder interface {
	RecordDependencyCheck(name string, healthy bool, d time.Duration)
}

// AggregatorOptions configures an Aggregator.
type AggregatorOptions struct {
	Service  string
	Version  string
	Checkers []Checker
	// Timeout bounds each individual dependency check.
	Timeout time.Duration
	// CacheTTL is the minimum interval between check rounds. Without it, a
	// probe every second across N replicas becomes N dependency queries per
	// second, and F09's benchmark would be measuring its own probe traffic.
	CacheTTL time.Duration
	Logger   *slog.Logger
	Metrics  MetricsRecorder
}

// Aggregator evaluates readiness across every dependency.
type Aggregator struct {
	opts AggregatorOptions

	sf singleflight.Group

	mu     sync.RWMutex
	cached *Result

	shuttingDown atomic.Bool
	checkRounds  atomic.Int64
}

// NewAggregator returns an Aggregator over the given checkers.
func NewAggregator(opts AggregatorOptions) *Aggregator {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return &Aggregator{opts: opts}
}

// BeginShutdown marks the process as shutting down. From this point readiness
// reports not-ready immediately and unconditionally, without running checks and
// without waiting for the cache to expire, so that traffic drains before
// connections start closing (FR-020). The transition is one-way.
func (a *Aggregator) BeginShutdown() { a.shuttingDown.Store(true) }

// CheckRounds reports how many times the dependencies have actually been
// probed. Tests use it to verify that probe frequency does not amplify load.
func (a *Aggregator) CheckRounds() int64 { return a.checkRounds.Load() }

// Evaluate returns the current readiness result, reusing a cached evaluation
// that is younger than CacheTTL. Concurrent callers that arrive during a check
// round share its result rather than starting their own.
func (a *Aggregator) Evaluate(ctx context.Context) Result {
	if a.shuttingDown.Load() {
		return a.shutdownResult()
	}

	if cached, ok := a.fresh(); ok {
		return cached
	}

	v, _, _ := a.sf.Do("evaluate", func() (any, error) {
		// A caller may have been waiting on an in-flight round that already
		// refreshed the cache; do not start a second one.
		if cached, ok := a.fresh(); ok {
			return cached, nil
		}
		result := a.evaluateNow(ctx)
		a.mu.Lock()
		a.cached = &result
		a.mu.Unlock()
		return result, nil
	})

	result, ok := v.(Result)
	if !ok {
		// singleflight only ever carries what the function above returned.
		return a.shutdownResult()
	}
	if a.shuttingDown.Load() {
		// Shutdown began while the round was running.
		return a.shutdownResult()
	}
	return result
}

// fresh returns the cached result when it is younger than CacheTTL.
func (a *Aggregator) fresh() (Result, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.cached == nil {
		return Result{}, false
	}
	if time.Since(a.cached.EvaluatedAt) >= a.opts.CacheTTL {
		return Result{}, false
	}
	return *a.cached, true
}

// evaluateNow probes every dependency concurrently. Running them in parallel
// keeps worst-case latency at one timeout rather than the sum of three, and
// checking all of them rather than stopping at the first failure is what lets
// the response name every failing dependency (FR-008).
func (a *Aggregator) evaluateNow(ctx context.Context) Result {
	a.checkRounds.Add(1)

	statuses := make([]Status, len(a.opts.Checkers))
	var wg sync.WaitGroup

	for i, checker := range a.opts.Checkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = run(ctx, checker, a.opts.Timeout)
		}()
	}
	wg.Wait()

	result := Result{
		Status:       StatusReady,
		Service:      a.opts.Service,
		Version:      a.opts.Version,
		EvaluatedAt:  time.Now().UTC(),
		Dependencies: statuses,
	}

	for _, st := range statuses {
		if a.opts.Metrics != nil {
			a.opts.Metrics.RecordDependencyCheck(st.Name, st.Healthy, time.Duration(st.DurationMS)*time.Millisecond)
		}
		if st.Healthy {
			continue
		}
		result.Status = StatusNotReady
		// The full error goes to the log, never to the HTTP response.
		a.opts.Logger.ErrorContext(ctx, "dependency_check_failed",
			slog.String("dependency", st.Name),
			slog.String("error_class", string(st.Reason)),
			slog.Int64("duration_ms", st.DurationMS),
			logging.Error(st.Err),
		)
	}

	return result
}

// shutdownResult reports not-ready without consulting any dependency.
func (a *Aggregator) shutdownResult() Result {
	deps := make([]Status, 0, len(a.opts.Checkers))

	a.mu.RLock()
	cached := a.cached
	a.mu.RUnlock()

	if cached != nil {
		deps = append(deps, cached.Dependencies...)
	} else {
		for _, c := range a.opts.Checkers {
			deps = append(deps, Status{Name: c.Name()})
		}
	}

	return Result{
		Status:       StatusNotReady,
		Service:      a.opts.Service,
		Version:      a.opts.Version,
		EvaluatedAt:  time.Now().UTC(),
		ShuttingDown: true,
		Dependencies: deps,
	}
}
