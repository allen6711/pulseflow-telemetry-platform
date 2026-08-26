package lifecycle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// recordingExit captures the exit code instead of terminating the test binary.
type recordingExit struct {
	mu     sync.Mutex
	called bool
	code   int
}

func (r *recordingExit) fn(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.called = true
	r.code = code
}

func (r *recordingExit) result() (bool, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.called, r.code
}

func TestStepsRunInOrder(t *testing.T) {
	var order []string
	var mu sync.Mutex
	record := func(name string) Step {
		return Step{Name: name, Fn: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return nil
		}}
	}

	err := Shutdown(
		Options{GracePeriod: time.Second, Logger: discardLogger()},
		record("readiness_gate"), record("http_server"), record("dependency_clients"),
	)
	if err != nil {
		t.Fatalf("Shutdown returned %v", err)
	}

	want := []string{"readiness_gate", "http_server", "dependency_clients"}
	if len(order) != len(want) {
		t.Fatalf("ran %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("step %d = %q, want %q (full order: %v)", i, order[i], want[i], order)
		}
	}
}

func TestStepFailureStopsTheSequence(t *testing.T) {
	boom := errors.New("server refused to stop")
	var reachedLast bool

	err := Shutdown(
		Options{GracePeriod: time.Second, Logger: discardLogger()},
		Step{Name: "first", Fn: func(context.Context) error { return boom }},
		Step{Name: "second", Fn: func(context.Context) error { reachedLast = true; return nil }},
	)

	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want %v", err, boom)
	}
	if reachedLast {
		t.Error("the sequence continued past a failing step")
	}
}

// FR-021: exceeding the grace period forces an exit rather than hanging.
func TestGracePeriodExpiryForcesExit(t *testing.T) {
	exit := &recordingExit{}

	err := Shutdown(
		Options{
			GracePeriod: 50 * time.Millisecond,
			Logger:      discardLogger(),
			Exit:        exit.fn,
		},
		Step{Name: "slow", Fn: func(ctx context.Context) error {
			<-ctx.Done()
			// A step that ignores its context entirely is the case this
			// bound exists for, so keep blocking past cancellation.
			time.Sleep(500 * time.Millisecond)
			return nil
		}},
	)

	if !errors.Is(err, ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
	called, code := exit.result()
	if !called {
		t.Fatal("the grace period expired without forcing an exit")
	}
	if code != ForcedExitCode {
		t.Errorf("exit code = %d, want %d", code, ForcedExitCode)
	}
}

// FR-023: a repeated signal exits immediately rather than re-running cleanup.
func TestSecondSignalExitsImmediatelyWithoutRepeatingCleanup(t *testing.T) {
	exit := &recordingExit{}
	second := make(chan os.Signal, 1)

	var runs int
	var mu sync.Mutex

	go func() {
		time.Sleep(20 * time.Millisecond)
		second <- syscall.SIGTERM
	}()

	err := Shutdown(
		Options{
			GracePeriod: 5 * time.Second,
			Logger:      discardLogger(),
			Second:      second,
			Exit:        exit.fn,
		},
		Step{Name: "slow", Fn: func(context.Context) error {
			mu.Lock()
			runs++
			mu.Unlock()
			time.Sleep(300 * time.Millisecond)
			return nil
		}},
	)
	if err != nil {
		t.Errorf("error = %v, want nil after a forced exit", err)
	}

	called, code := exit.result()
	if !called {
		t.Fatal("the second signal did not force an exit")
	}
	if code != ForcedExitCode {
		t.Errorf("exit code = %d, want %d", code, ForcedExitCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if runs != 1 {
		t.Errorf("the cleanup step ran %d times, want exactly 1", runs)
	}
}

func TestNormalCompletionDoesNotForceExit(t *testing.T) {
	exit := &recordingExit{}

	err := Shutdown(
		Options{GracePeriod: time.Second, Logger: discardLogger(), Exit: exit.fn},
		Step{Name: "quick", Fn: func(context.Context) error { return nil }},
	)
	if err != nil {
		t.Fatalf("Shutdown returned %v", err)
	}
	if called, _ := exit.result(); called {
		t.Error("a clean shutdown must not force an exit")
	}
}
