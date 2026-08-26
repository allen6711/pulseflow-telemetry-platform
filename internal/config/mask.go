package config

import (
	"fmt"
	"log/slog"
	"strings"
)

// Masked is what a sensitive value renders as.
const Masked = "***"

// mask replaces a non-empty secret with a fixed placeholder. Empty stays empty,
// so a reader can tell "no password configured" from "password withheld".
func mask(secret string) string {
	if secret == "" {
		return ""
	}
	return Masked
}

// String renders the configuration for logging and diagnostics with every
// sensitive value masked (FR-018, FR-030).
//
// It is a method on the value, not a formatting helper, so that any code path
// that prints a Config -- including one added later by mistake -- masks by
// default rather than by remembering to.
func (c Config) String() string {
	var b strings.Builder
	b.WriteString("Config{")

	fields := []struct {
		name  string
		value any
	}{
		{"service_name", c.ServiceName},
		{"version", c.Version},
		{"environment", c.Environment},
		{"http_port", c.HTTPPort},
		{"shutdown_grace_period", c.ShutdownGracePeriod},
		{"log_level", c.LogLevel},
		{"kafka_brokers", strings.Join(c.KafkaBrokers, ",")},
		{"clickhouse_addr", c.ClickHouseAddr},
		{"clickhouse_database", c.ClickHouseDatabase},
		{"clickhouse_user", c.ClickHouseUser},
		{"clickhouse_password", mask(c.ClickHousePassword)},
		{"redis_addr", c.RedisAddr},
		{"redis_password", mask(c.RedisPassword)},
		{"health_check_timeout", c.HealthCheckTimeout},
		{"health_cache_ttl", c.HealthCacheTTL},
	}

	for i, f := range fields {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%s=%v", f.name, f.value)
	}

	b.WriteString("}")
	return b.String()
}

// LogValue implements slog.LogValuer so that passing a Config to a logger emits
// the masked rendering rather than the struct's raw fields.
//
// The signature matters: slog only honours LogValue() slog.Value. Any other
// return type is ignored, the struct is reflected instead, and every secret it
// holds lands in the log.
func (c Config) LogValue() slog.Value { return slog.StringValue(c.String()) }

// Compile-time guarantee that Config satisfies slog.LogValuer. Without it, a
// signature drift would silently fall back to reflecting the struct.
var _ slog.LogValuer = Config{}
