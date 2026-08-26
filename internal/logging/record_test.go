package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// requiredFields must appear on every record, per contracts/log-record.md.
var requiredFields = []string{"time", "level", "msg", "service", "version", "trace_id"}

// SC-007: every emitted record parses independently and carries all required
// fields.
func TestEveryRecordCarriesTheRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{ServiceName: "pulseflow-api", Version: "v1.2.3", Level: "debug"})

	ctx := ContextWithTraceID(context.Background(), NewTraceID())
	logger.DebugContext(ctx, "http_request", "route", "/v1/health/ready", "code", 200)
	logger.InfoContext(context.Background(), "service_started", "port", 8080)
	logger.WarnContext(ctx, "redis_client", "detail", "pool timeout")
	logger.ErrorContext(ctx, "dependency_check_failed",
		"dependency", "clickhouse", "error_class", "timeout", Error(errors.New("ping failed")))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("emitted %d records, want 4:\n%s", len(lines), buf.String())
	}

	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("record %d does not parse independently: %v\n%s", i, err, line)
			continue
		}
		for _, field := range requiredFields {
			if _, ok := rec[field]; !ok {
				t.Errorf("record %d is missing %q: %s", i, field, line)
			}
		}
		if id, _ := rec["trace_id"].(string); len(id) != TraceIDLen*2 {
			t.Errorf("record %d trace_id = %q, want %d hex characters", i, id, TraceIDLen*2)
		}
	}
}

// SC-008: one request's records are all retrievable by trace id alone.
func TestAllRecordsFromOneRequestShareTheTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{ServiceName: "svc", Version: "v1", Level: "debug"})

	id := NewTraceID()
	ctx := ContextWithTraceID(context.Background(), id)

	logger.InfoContext(ctx, "http_request")
	logger.With("dependency", "redis").ErrorContext(ctx, "dependency_check_failed")
	logger.DebugContext(ctx, "http_request")
	// An unrelated record that must not be picked up by the same query.
	logger.InfoContext(context.Background(), "shutdown_started")

	var matched int
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unparseable record: %v", err)
		}
		if rec["trace_id"] == id.String() {
			matched++
		}
	}

	if matched != 3 {
		t.Errorf("retrieved %d records by trace id, want exactly the 3 from that request", matched)
	}
}

// Content that would break a line-oriented parser must still round-trip.
func TestAwkwardContentStaysParseable(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{ServiceName: "svc", Version: "v1", Level: "info"})

	logger.InfoContext(context.Background(), "event",
		"detail", "line one\nline two \"quoted\" and a tab\there")

	if got := strings.Count(strings.TrimSpace(buf.String()), "\n"); got != 0 {
		t.Errorf("the record spans %d extra lines; embedded newlines must be escaped", got)
	}
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Errorf("record with awkward content does not parse: %v\n%s", err, buf.String())
	}
}
