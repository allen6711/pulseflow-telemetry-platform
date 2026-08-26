package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// FR-028: records below the configured level are not emitted.
func TestLevelFiltering(t *testing.T) {
	cases := []struct {
		configured string
		emitted    []string
		suppressed []string
	}{
		{"debug", []string{"DEBUG", "INFO", "WARN", "ERROR"}, nil},
		{"info", []string{"INFO", "WARN", "ERROR"}, []string{"DEBUG"}},
		{"warn", []string{"WARN", "ERROR"}, []string{"DEBUG", "INFO"}},
		{"error", []string{"ERROR"}, []string{"DEBUG", "INFO", "WARN"}},
	}

	for _, tc := range cases {
		t.Run(tc.configured, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(&buf, Options{ServiceName: "svc", Version: "v1", Level: tc.configured})

			ctx := context.Background()
			logger.DebugContext(ctx, "debug_event")
			logger.InfoContext(ctx, "info_event")
			logger.WarnContext(ctx, "warn_event")
			logger.ErrorContext(ctx, "error_event")

			out := buf.String()
			for _, level := range tc.emitted {
				if !strings.Contains(out, `"level":"`+level+`"`) {
					t.Errorf("level %s should be emitted at %q:\n%s", level, tc.configured, out)
				}
			}
			for _, level := range tc.suppressed {
				if strings.Contains(out, `"level":"`+level+`"`) {
					t.Errorf("level %s should be suppressed at %q:\n%s", level, tc.configured, out)
				}
			}
		})
	}
}

func TestParseLevelRejectsUnknownNames(t *testing.T) {
	for _, name := range []string{"debug", "INFO", " warn ", "error", "warning"} {
		if _, ok := ParseLevel(name); !ok {
			t.Errorf("ParseLevel(%q) rejected a valid level", name)
		}
	}
	for _, name := range []string{"verbose", "trace", "", "critical"} {
		if _, ok := ParseLevel(name); ok {
			t.Errorf("ParseLevel(%q) accepted an unknown level", name)
		}
	}
}

// Timestamps follow contracts/log-record.md: RFC3339 with milliseconds, UTC.
func TestTimestampFormat(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, Options{ServiceName: "svc", Version: "v1", Level: "info"}).
		InfoContext(context.Background(), "event")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("record is not JSON: %v", err)
	}

	ts, _ := rec[slog.TimeKey].(string)
	if !strings.HasSuffix(ts, "Z") {
		t.Errorf("time = %q, want a UTC timestamp ending in Z", ts)
	}
	// 2026-08-27T02:00:00.000Z
	if len(ts) != 24 {
		t.Errorf("time = %q (%d chars), want RFC3339 with milliseconds", ts, len(ts))
	}
}
