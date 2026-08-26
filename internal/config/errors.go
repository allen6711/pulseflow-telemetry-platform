package config

import (
	"fmt"
	"strings"
)

// ValidationError reports every configuration problem found, not just the
// first.
//
// Reporting one problem at a time turns an environment with three mistakes into
// three edit-restart cycles, which is what SC-005 exists to rule out.
type ValidationError struct {
	Problems []Problem
}

// Error renders one line per failing setting. Each line names the variable,
// quotes the value received, and states the permitted format or range, so a
// reader can tell what to change without opening the source (FR-016).
func (e *ValidationError) Error() string {
	if len(e.Problems) == 0 {
		return "configuration invalid"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "configuration invalid (%d %s):",
		len(e.Problems), plural(len(e.Problems), "error", "errors"))
	for _, p := range e.Problems {
		b.WriteString("\n  ")
		b.WriteString(p.String())
	}
	return b.String()
}

// Fields returns the names of the settings that failed, for structured logging.
func (e *ValidationError) Fields() []string {
	out := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		out = append(out, p.Variable)
	}
	return out
}

// Messages returns one rendered message per failing setting.
func (e *ValidationError) Messages() []string {
	out := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		out = append(out, p.String())
	}
	return out
}
