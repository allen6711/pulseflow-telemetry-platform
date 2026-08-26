package logging

import (
	"context"
	"log/slog"
)

// TraceIDField is the log field carrying the correlation identifier.
const TraceIDField = "trace_id"

// contextHandler injects the context's correlation identifier into every
// record, so call sites never pass trace_id explicitly and cannot forget to.
type contextHandler struct {
	inner slog.Handler
}

// NewContextHandler wraps a handler so that every record it emits carries a
// trace_id field.
func NewContextHandler(inner slog.Handler) slog.Handler {
	return contextHandler{inner: inner}
}

func (h contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(slog.String(TraceIDField, TraceIDFromContext(ctx).String()))
	return h.inner.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{inner: h.inner.WithGroup(name)}
}
