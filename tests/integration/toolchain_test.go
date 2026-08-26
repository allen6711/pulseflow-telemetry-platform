//go:build integration

package integration

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// FR-036: the pipeline's checks must be reproducible locally through a single
// command. That only holds if CI invokes the same Makefile targets a
// contributor runs, rather than duplicating the commands inline where the two
// can drift apart.
func TestCIRunsTheSameTargetsAsMakeCheck(t *testing.T) {
	workflow := readFile(t, repoRoot(t)+"/.github/workflows/ci.yml")
	makefile := readFile(t, repoRoot(t)+"/Makefile")

	// Everything `make check` aggregates.
	checkTargets := targetsInvokedBy(t, makefile, "check")
	if len(checkTargets) == 0 {
		t.Fatal("make check does not aggregate any targets")
	}

	for _, target := range checkTargets {
		if !strings.Contains(workflow, "make "+target) {
			t.Errorf("`make check` runs %q but no CI job invokes `make %s`; "+
				"the two will drift", target, target)
		}
	}

	// The reverse direction: a CI job that runs a raw go command instead of a
	// Makefile target is exactly the drift this test guards against.
	rawGo := regexp.MustCompile(`(?m)^\s+run:\s+go\s+(build|test|vet)\b`)
	if loc := rawGo.FindString(workflow); loc != "" {
		t.Errorf("a CI step runs a raw go command instead of a Makefile target: %q", strings.TrimSpace(loc))
	}
}

func TestMakeCheckCoversBuildLintAndTest(t *testing.T) {
	makefile := readFile(t, repoRoot(t)+"/Makefile")

	for _, want := range []string{"build", "lint", "test"} {
		if !containsTarget(makefile, want) {
			t.Errorf("the Makefile has no %q target", want)
		}
	}

	targets := targetsInvokedBy(t, makefile, "check")
	for _, want := range []string{"build", "lint", "test"} {
		var found bool
		for _, got := range targets {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("`make check` does not run %q; it aggregates %v", want, targets)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

func containsTarget(makefile, target string) bool {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:`).MatchString(makefile)
}

// targetsInvokedBy returns the targets a rule delegates to via $(MAKE).
func targetsInvokedBy(t *testing.T, makefile, target string) []string {
	t.Helper()

	rule := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(target) + `:.*?\n(?:\t.*\n|\n)*`)
	body := rule.FindString(makefile)
	if body == "" {
		t.Fatalf("the Makefile has no %q target", target)
	}

	var out []string
	for _, m := range regexp.MustCompile(`\$\(MAKE\)\s+(\S+)`).FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}
