package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Bounds shared between validation and its error messages.
const (
	minPort          = 1
	maxPort          = 65535
	maxGracePeriod   = 5 * time.Minute
	maxHealthTimeout = 30 * time.Second
)

// validEnvironments and validLogLevels are the permitted enumerations.
var (
	validEnvironments = []string{"local", "ci", "benchmark"}
	validLogLevels    = []string{"debug", "info", "warn", "error"}
)

// validate applies the semantic rules parsing alone cannot catch: permitted
// ranges, enumerations, and rules spanning more than one field.
func validate(cfg Config, c *collector) {
	requireNonEmpty(c, "SERVICE_NAME", cfg.ServiceName)
	requireNonEmpty(c, "VERSION", cfg.Version)
	requireOneOf(c, "ENVIRONMENT", cfg.Environment, validEnvironments)

	requireIntRange(c, "HTTP_PORT", cfg.HTTPPort, minPort, maxPort)
	requireDurationRange(c, "SHUTDOWN_GRACE_PERIOD", cfg.ShutdownGracePeriod, 0, maxGracePeriod, false)

	requireOneOf(c, "LOG_LEVEL", cfg.LogLevel, validLogLevels)

	if len(cfg.KafkaBrokers) == 0 {
		c.add("KAFKA_BROKERS", "", "must contain at least one host:port entry")
	}
	for _, broker := range cfg.KafkaBrokers {
		requireHostPort(c, "KAFKA_BROKERS", broker)
	}

	requireHostPort(c, "CLICKHOUSE_ADDR", cfg.ClickHouseAddr)
	requireNonEmpty(c, "CLICKHOUSE_DATABASE", cfg.ClickHouseDatabase)
	requireNonEmpty(c, "CLICKHOUSE_USER", cfg.ClickHouseUser)

	requireHostPort(c, "REDIS_ADDR", cfg.RedisAddr)

	requireDurationRange(c, "HEALTH_CHECK_TIMEOUT", cfg.HealthCheckTimeout, 0, maxHealthTimeout, false)
	requireDurationRange(c, "HEALTH_CACHE_TTL", cfg.HealthCacheTTL, 0, maxHealthTimeout, true)

	// Cross-field: a cached failure must not outlive the check that produced
	// it, or FR-011's recovery window becomes unbounded.
	if cfg.HealthCacheTTL >= cfg.HealthCheckTimeout {
		c.add("HEALTH_CACHE_TTL", cfg.HealthCacheTTL.String(),
			fmt.Sprintf("must be less than %sHEALTH_CHECK_TIMEOUT (%s)", EnvPrefix, cfg.HealthCheckTimeout))
	}
}

func requireNonEmpty(c *collector, name, value string) {
	if strings.TrimSpace(value) == "" {
		c.add(name, value, "must not be empty")
	}
}

func requireOneOf(c *collector, name, value string, allowed []string) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	c.add(name, value, "must be one of "+strings.Join(allowed, ", "))
}

func requireIntRange(c *collector, name string, value, minimum, maximum int) {
	if value < minimum || value > maximum {
		c.add(name, fmt.Sprint(value),
			fmt.Sprintf("must be an integer between %d and %d", minimum, maximum))
	}
}

// requireDurationRange checks 0 <= value <= maximum, allowing zero only when
// allowZero is set.
func requireDurationRange(c *collector, name string, value, _ time.Duration, maximum time.Duration, allowZero bool) {
	lower := "greater than 0"
	if allowZero {
		lower = "0 or greater"
	}
	if (value < 0) || (!allowZero && value == 0) || value > maximum {
		c.add(name, value.String(), fmt.Sprintf("must be %s and at most %s", lower, maximum))
	}
}

// requireHostPort checks that a value parses as host:port with a numeric port.
func requireHostPort(c *collector, name, value string) {
	const want = `must be host:port, for example "localhost:9092"`

	host, port, err := net.SplitHostPort(value)
	if err != nil || host == "" || port == "" {
		c.add(name, value, want)
		return
	}
	// A numeric port, not a service name: resolving /etc/services would make
	// configuration validity depend on the host it runs on.
	n, err := strconv.Atoi(port)
	if err != nil || n < minPort || n > maxPort {
		c.add(name, value, want)
	}
}
