package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// decodeOne parses the single JSON record written to buf.
func decodeOne(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("no log record was emitted")
	}
	if strings.Contains(line, "\n") {
		t.Fatalf("expected exactly one record, got:\n%s", line)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("record is not parseable JSON: %v\nrecord: %s", err, line)
	}
	return rec
}

func TestHandlerInjectsTraceIDFromContext(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{ServiceName: "svc", Version: "v1", Level: "info"})

	want := NewTraceID()
	ctx := ContextWithTraceID(context.Background(), want)
	logger.InfoContext(ctx, "some_event")

	rec := decodeOne(t, &buf)
	if got := rec[TraceIDField]; got != want.String() {
		t.Errorf("trace_id = %v, want %v", got, want)
	}
}

func TestHandlerUsesZeroTraceIDWithoutContext(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{ServiceName: "svc", Version: "v1", Level: "info"})

	logger.InfoContext(context.Background(), "startup_event")

	rec := decodeOne(t, &buf)
	got, ok := rec[TraceIDField].(string)
	if !ok {
		t.Fatalf("trace_id missing; every record must carry the field, got %#v", rec)
	}
	if got != zeroTraceID.String() {
		t.Errorf("trace_id = %q, want the all-zero id %q", got, zeroTraceID)
	}
	if len(got) != TraceIDLen*2 {
		t.Errorf("trace_id length = %d, want %d hex characters", len(got), TraceIDLen*2)
	}
}

func TestHandlerKeepsTraceIDAcrossWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{ServiceName: "svc", Version: "v1", Level: "info"})

	want := NewTraceID()
	ctx := ContextWithTraceID(context.Background(), want)
	logger.With("dependency", "clickhouse").InfoContext(ctx, "dependency_check_failed")

	rec := decodeOne(t, &buf)
	if got := rec[TraceIDField]; got != want.String() {
		t.Errorf("trace_id after With() = %v, want %v", got, want)
	}
	if got := rec["dependency"]; got != "clickhouse" {
		t.Errorf("dependency = %v, want clickhouse", got)
	}
}

func TestParseTraceIDRejectsMalformedAndZero(t *testing.T) {
	valid := NewTraceID()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid", valid.String(), true},
		{"too short", "abcd", false},
		{"too long", valid.String() + "00", false},
		{"non hex", strings.Repeat("z", 32), false},
		{"all zero", strings.Repeat("0", 32), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseTraceID(tc.in)
			if ok != tc.want {
				t.Fatalf("ParseTraceID(%q) ok = %v, want %v", tc.in, ok, tc.want)
			}
			if ok && got.String() != tc.in {
				t.Errorf("round trip = %q, want %q", got, tc.in)
			}
		})
	}
}
