//go:build integration

package integration

import (
	"strings"
	"testing"
)

// TestRepeatedUpDownIsIdempotent verifies FR-004: running the start command
// twice produces the same result, with no residual state carried between runs.
//
// It is disruptive -- it tears the stack down and brings it back up -- so it is
// opt-in. Set PULSEFLOW_DISRUPTIVE_TESTS=1 to run it. scripts/verify-restart-
// idempotence.sh performs the same check over five cycles for SC-010.
func TestRepeatedUpDownIsIdempotent(t *testing.T) {
	if envOr("PULSEFLOW_DISRUPTIVE_TESTS", "") != "1" {
		t.Skip("disruptive: set PULSEFLOW_DISRUPTIVE_TESTS=1 to run")
	}

	for cycle := 1; cycle <= 2; cycle++ {
		if out, err := compose(t, "down", "--remove-orphans", "--volumes"); err != nil {
			t.Fatalf("cycle %d: compose down: %v\n%s", cycle, err, out)
		}
		if out, err := compose(t, "up", "-d", "--build", "--wait"); err != nil {
			t.Fatalf("cycle %d: compose up did not reach healthy: %v\n%s", cycle, err, out)
		}

		for _, svc := range supportingServices {
			if got := serviceHealth(t, svc); got != "healthy" {
				t.Errorf("cycle %d: %s health = %q, want healthy", cycle, svc, got)
			}
		}
	}
}

// TestDownLeavesNoResidualState verifies that a stopped stack leaves nothing
// behind that would change the outcome of the next start.
func TestDownLeavesNoResidualState(t *testing.T) {
	if envOr("PULSEFLOW_DISRUPTIVE_TESTS", "") != "1" {
		t.Skip("disruptive: set PULSEFLOW_DISRUPTIVE_TESTS=1 to run")
	}

	if out, err := compose(t, "down", "--remove-orphans", "--volumes"); err != nil {
		t.Fatalf("compose down: %v\n%s", err, out)
	}

	out, err := compose(t, "ps", "--all", "--quiet")
	if err != nil {
		t.Fatalf("compose ps: %v\n%s", err, out)
	}
	if remaining := strings.TrimSpace(out); remaining != "" {
		t.Errorf("containers remain after down:\n%s", remaining)
	}

	// Restore the stack for any test that runs after this one.
	if out, err := compose(t, "up", "-d", "--build", "--wait"); err != nil {
		t.Fatalf("restoring the stack: %v\n%s", err, out)
	}
}
