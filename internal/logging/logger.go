package logging

import (
	"io"
	"log/slog"
	"strings"
	"time"
)

// Options configures the logger. ServiceName and Version are attached to every
// record; Level filters records below it.
type Options struct {
	ServiceName string
	Version     string
	Level       string
}

// ParseLevel maps a configured level name to a slog level. It reports false for
// unrecognized names so that configuration validation, not the logger, decides
// what to do about them.
func ParseLevel(name string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// New returns a logger emitting one JSON object per line, with the field set
// fixed by contracts/log-record.md: time, level, msg, service, version, and
// trace_id on every record.
func New(w io.Writer, opts Options) *slog.Logger {
	level, _ := ParseLevel(opts.Level)

	base := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceAttr,
	})

	return slog.New(NewContextHandler(base)).With(
		slog.String("service", opts.ServiceName),
		slog.String("version", opts.Version),
	)
}

// replaceAttr renders timestamps as RFC3339 with milliseconds in UTC, which is
// what contracts/log-record.md specifies. slog's default is RFC3339Nano in
// local time.
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.StringValue(t.UTC().Format("2006-01-02T15:04:05.000Z"))
		}
	}
	return a
}
