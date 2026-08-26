package lifecycle

import (
	"context"
	"testing"
	"time"
)

// FR-022: a signal arriving before startup finishes must abort safely rather
// than leave a half-initialized process behind.
func TestAbortedReportsAPreEmptedStartup(t *testing.T) {
	live, cancel := context.WithCancel(context.Background())
	defer cancel()

	if Aborted(live) {
		t.Error("a live context must not report an abort")
	}

	cancel()

	if !Aborted(live) {
		t.Error("a cancelled context must report an abort")
	}
}

func TestSignalsCancelTheRootContext(t *testing.T) {
	ctx, second, stop := Signals(context.Background())
	defer stop()

	if second == nil {
		t.Fatal("Signals must return a channel for the repeated signal")
	}
	if ctx.Err() != nil {
		t.Fatal("the context must start live")
	}

	// Cancelling the parent stands in for a signal: what matters here is that
	// callers observe cancellation through the returned context.
	parent, cancelParent := context.WithCancel(context.Background())
	derived, _, stopDerived := Signals(parent)
	defer stopDerived()

	cancelParent()

	select {
	case <-derived.Done():
	case <-time.After(time.Second):
		t.Error("the derived context did not cancel with its parent")
	}
	if !Aborted(derived) {
		t.Error("Aborted must report the cancelled context")
	}
}
