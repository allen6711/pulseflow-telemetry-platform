package logging

import (
	"context"
	"fmt"
	"log/slog"
)

// redisAdapter routes go-redis's internal logging into slog.
//
// go-redis writes connection-pool diagnostics to its own package-level logger,
// which by default prints unstructured lines to stderr. That breaks FR-024:
// every log line must be independently parseable. Routing them here keeps the
// output uniform and lets the configured level filter them.
type redisAdapter struct {
	log *slog.Logger
}

func (a redisAdapter) Printf(ctx context.Context, format string, v ...any) {
	a.log.WarnContext(ctx, "redis_client",
		slog.String("error_class", "protocol_error"),
		slog.String("detail", fmt.Sprintf(format, v...)),
	)
}

// NewRedisLogger returns a logger adapter suitable for redis.SetLogger.
func NewRedisLogger(log *slog.Logger) interface {
	Printf(ctx context.Context, format string, v ...any)
} {
	return redisAdapter{log: log}
}
