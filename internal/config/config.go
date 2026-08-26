// Package config loads PulseFlow service configuration from the environment.
//
// Every setting has a default that works for local development, and the whole
// configuration is validated at startup: a service either starts with a valid,
// immutable configuration or fails before opening a listener. The contract of
// record is specs/001-platform-foundation/contracts/configuration.md.
package config

import "time"

// EnvPrefix is prepended to every environment variable this package reads.
const EnvPrefix = "PULSEFLOW_"

// Config is the validated, immutable configuration for one service process.
type Config struct {
	// Identity.
	ServiceName string
	Version     string
	Environment string

	// HTTP server.
	HTTPPort            int
	ShutdownGracePeriod time.Duration

	// Logging.
	LogLevel string

	// Kafka.
	KafkaBrokers []string

	// ClickHouse.
	ClickHouseAddr     string
	ClickHouseDatabase string
	ClickHouseUser     string
	ClickHousePassword string // sensitive

	// Redis.
	RedisAddr     string
	RedisPassword string // sensitive

	// Health checking.
	HealthCheckTimeout time.Duration
	HealthCacheTTL     time.Duration
}

// ServiceDefaults carries the two defaults that differ between the two binaries.
type ServiceDefaults struct {
	ServiceName string
	HTTPPort    int
}

// Defaults for each binary.
var (
	APIDefaults    = ServiceDefaults{ServiceName: "pulseflow-api", HTTPPort: 8080}
	WorkerDefaults = ServiceDefaults{ServiceName: "pulseflow-worker", HTTPPort: 8081}
)

// Load reads configuration from the environment, applies defaults, and validates
// the result. It reports every problem it finds rather than stopping at the
// first, so a developer with three mistakes sees three messages once instead of
// one message three restarts in a row.
func Load(d ServiceDefaults) (Config, error) {
	c := &collector{}

	cfg := Config{
		ServiceName: c.str("SERVICE_NAME", d.ServiceName),
		Version:     c.str("VERSION", "dev"),
		Environment: c.str("ENVIRONMENT", "local"),

		HTTPPort:            c.intVal("HTTP_PORT", d.HTTPPort),
		ShutdownGracePeriod: c.duration("SHUTDOWN_GRACE_PERIOD", 30*time.Second),

		LogLevel: c.str("LOG_LEVEL", "info"),

		KafkaBrokers: c.list("KAFKA_BROKERS", []string{"localhost:9092"}),

		ClickHouseAddr:     c.str("CLICKHOUSE_ADDR", "localhost:9000"),
		ClickHouseDatabase: c.str("CLICKHOUSE_DATABASE", "pulseflow"),
		ClickHouseUser:     c.str("CLICKHOUSE_USER", "default"),
		ClickHousePassword: c.str("CLICKHOUSE_PASSWORD", ""),

		RedisAddr:     c.str("REDIS_ADDR", "localhost:6379"),
		RedisPassword: c.str("REDIS_PASSWORD", ""),

		HealthCheckTimeout: c.duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
		HealthCacheTTL:     c.duration("HEALTH_CACHE_TTL", time.Second),
	}

	validate(cfg, c)

	if len(c.problems) > 0 {
		return Config{}, &ValidationError{Problems: c.problems}
	}
	return cfg, nil
}
