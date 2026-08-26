// Package lifecycle owns process startup and shutdown ordering.
//
// Predictable termination is not a nicety here: F05's worker restart drills and
// F09's failure experiment both rest on it. If shutdown behaviour is uncertain,
// the results of those experiments cannot be interpreted.
package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// DefaultSignals are the termination signals both binaries respond to.
var DefaultSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// Signals returns a context cancelled by the first termination signal, plus a
// channel that receives the second one.
//
// The second signal matters in practice: a developer who presses Ctrl-C twice
// expects the process to die, and an orchestrator sends SIGKILL after the grace
// period anyway. A process that ignores repeated signals just looks hung.
func Signals(parent context.Context, sigs ...os.Signal) (ctx context.Context, second <-chan os.Signal, stop func()) {
	if len(sigs) == 0 {
		sigs = DefaultSignals
	}

	ctx, cancel := signal.NotifyContext(parent, sigs...)

	repeat := make(chan os.Signal, 1)
	go func() {
		<-ctx.Done()
		// Re-arm only after the first signal, so the channel carries the
		// second one and nothing earlier.
		signal.Notify(repeat, sigs...)
	}()

	return ctx, repeat, func() {
		signal.Stop(repeat)
		cancel()
	}
}

// Step is one named stage of shutdown.
type Step struct {
	Name string
	Fn   func(context.Context) error
}

// Options configures Shutdown.
type Options struct {
	// GracePeriod bounds the whole sequence.
	GracePeriod time.Duration
	Logger      *slog.Logger
	// Second receives a repeated termination signal, which forces an
	// immediate exit rather than a second cleanup pass.
	Second <-chan os.Signal
	// Exit is called on the forced paths. Defaults to os.Exit; tests replace it.
	Exit func(code int)
	// Signal names the signal that started the shutdown, for the log event.
	Signal string
}

// ForcedExitCode is the status used when shutdown is cut short.
const ForcedExitCode = 1

// ErrTimeout reports that the grace period expired before shutdown finished.
var ErrTimeout = errors.New("shutdown exceeded its grace period")

// Shutdown runs the steps in order, bounded by the grace period.
//
// The order is the contract: the readiness gate flips first so traffic drains,
// then the server stops accepting and waits for in-flight requests, then
// clients close. Flipping readiness after the server has begun closing would
// race the load balancer and drop requests that were already in flight.
func Shutdown(opts Options, steps ...Step) error {
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	log.Info("shutdown_started",
		slog.String("signal", opts.Signal),
		slog.String("grace_period", opts.GracePeriod.String()),
	)

	ctx, cancel := context.WithTimeout(context.Background(), opts.GracePeriod)
	defer cancel()

	exit := opts.Exit
	if exit == nil {
		exit = os.Exit
	}

	start := time.Now()
	done := make(chan error, 1)

	go func() {
		for _, step := range steps {
			if err := step.Fn(ctx); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			return err
		}
		log.Info("shutdown_complete", slog.Int64("duration_ms", time.Since(start).Milliseconds()))
		return nil

	case sig := <-opts.Second:
		// A repeated signal means "stop now", not "clean up again".
		log.Warn("shutdown_forced",
			slog.String("signal", signalName(sig)),
			slog.Int64("elapsed_ms", time.Since(start).Milliseconds()),
		)
		exit(ForcedExitCode)
		return nil

	case <-ctx.Done():
		log.Error("shutdown_timeout",
			slog.String("grace_period", opts.GracePeriod.String()),
			slog.String("error_class", "timeout"),
			slog.Int64("elapsed_ms", time.Since(start).Milliseconds()),
		)
		exit(ForcedExitCode)
		return ErrTimeout
	}
}

// Aborted reports whether a termination signal arrived before startup finished.
//
// A process that is killed mid-startup must exit cleanly rather than leave a
// half-initialized state behind, so callers check this before they begin
// serving.
func Aborted(ctx context.Context) bool {
	return ctx.Err() != nil
}

func signalName(sig os.Signal) string {
	if sig == nil {
		return "unknown"
	}
	return sig.String()
}
