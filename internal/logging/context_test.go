package logging

import (
	"context"
	"strings"
	"testing"
)

func TestTraceparentAdoption(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

	cases := []struct {
		name   string
		header string
		want   string // empty means "a fresh id, not this one"
	}{
		{"valid traceparent", "00-" + traceID + "-00f067aa0ba902b7-01", traceID},
		{"sampled flag off", "00-" + traceID + "-00f067aa0ba902b7-00", traceID},
		{"future version", "01-" + traceID + "-00f067aa0ba902b7-01-extra", traceID},
		{"missing header", "", ""},
		{"malformed", "not-a-traceparent", ""},
		{"too few segments", "00-" + traceID, ""},
		{"all-zero trace id", "00-" + strings.Repeat("0", 32) + "-00f067aa0ba902b7-01", ""},
		{"short trace id", "00-abcd-00f067aa0ba902b7-01", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TraceIDForRequest(tc.header)

			if tc.want != "" {
				if got.String() != tc.want {
					t.Errorf("trace id = %q, want the inbound %q", got, tc.want)
				}
				return
			}

			// FR-027: a request without a usable trace id gets a fresh one.
			if got.IsZero() {
				t.Error("a fresh trace id must be generated, not the all-zero id")
			}
			if len(got.String()) != TraceIDLen*2 {
				t.Errorf("generated id is %d characters, want %d", len(got.String()), TraceIDLen*2)
			}
		})
	}
}

func TestGeneratedTraceIDsAreDistinct(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for range 1000 {
		id := TraceIDForRequest("").String()
		if seen[id] {
			t.Fatalf("generated a duplicate trace id: %s", id)
		}
		seen[id] = true
	}
}

func TestContextRoundTrip(t *testing.T) {
	want := NewTraceID()
	ctx := ContextWithTraceID(context.Background(), want)

	if got := TraceIDFromContext(ctx); got != want {
		t.Errorf("round trip = %q, want %q", got, want)
	}
	if got := TraceIDFromContext(context.Background()); !got.IsZero() {
		t.Errorf("a bare context yielded %q, want the all-zero id", got)
	}
}
