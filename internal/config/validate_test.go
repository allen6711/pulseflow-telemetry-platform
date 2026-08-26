package config

import (
	"errors"
	"strings"
	"testing"
)

// loadWith runs Load with only the given variables set.
func loadWith(t *testing.T, env map[string]string) (Config, error) {
	t.Helper()
	for k, v := range env {
		t.Setenv(EnvPrefix+k, v)
	}
	return Load(APIDefaults)
}

func TestDefaultsAreUsableWithNoEnvironmentSet(t *testing.T) {
	cfg, err := loadWith(t, nil)
	if err != nil {
		t.Fatalf("defaults must load cleanly, got: %v", err)
	}
	if cfg.ServiceName != APIDefaults.ServiceName {
		t.Errorf("service name = %q, want %q", cfg.ServiceName, APIDefaults.ServiceName)
	}
	if cfg.HTTPPort != APIDefaults.HTTPPort {
		t.Errorf("port = %d, want %d", cfg.HTTPPort, APIDefaults.HTTPPort)
	}
	if cfg.HealthCacheTTL >= cfg.HealthCheckTimeout {
		t.Error("the shipped defaults violate the cache TTL cross-field rule")
	}
}

// SC-005: three mistakes cost one restart, not three. Reporting only the first
// failure is what this test exists to prevent.
func TestEveryProblemIsReportedInOnePass(t *testing.T) {
	_, err := loadWith(t, map[string]string{
		"HTTP_PORT":        "http",
		"LOG_LEVEL":        "verbose",
		"HEALTH_CACHE_TTL": "5s",
	})
	if err == nil {
		t.Fatal("expected the load to fail")
	}

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("error is %T, want *ValidationError", err)
	}

	want := []string{
		EnvPrefix + "HTTP_PORT",
		EnvPrefix + "LOG_LEVEL",
		EnvPrefix + "HEALTH_CACHE_TTL",
	}
	got := invalid.Fields()
	if len(got) != len(want) {
		t.Fatalf("reported %d problems (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for _, w := range want {
		var found bool
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("%s was not reported; reported: %v", w, got)
		}
	}
}

func TestInvalidValuesAreRejected(t *testing.T) {
	cases := []struct {
		name     string
		variable string
		value    string
	}{
		{"non-numeric port", "HTTP_PORT", "http"},
		{"port below range", "HTTP_PORT", "0"},
		{"port above range", "HTTP_PORT", "70000"},
		{"unknown log level", "LOG_LEVEL", "verbose"},
		{"unknown environment", "ENVIRONMENT", "production"},
		{"malformed duration", "SHUTDOWN_GRACE_PERIOD", "thirty"},
		{"zero grace period", "SHUTDOWN_GRACE_PERIOD", "0s"},
		{"grace period above cap", "SHUTDOWN_GRACE_PERIOD", "10m"},
		{"health timeout above cap", "HEALTH_CHECK_TIMEOUT", "45s"},
		{"broker without port", "KAFKA_BROKERS", "localhost"},
		{"clickhouse without port", "CLICKHOUSE_ADDR", "clickhouse"},
		{"redis with non-numeric port", "REDIS_ADDR", "redis:six"},
		{"empty clickhouse database", "CLICKHOUSE_DATABASE", " "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadWith(t, map[string]string{tc.variable: tc.value})
			if err == nil {
				t.Fatalf("%s=%q was accepted", EnvPrefix+tc.variable, tc.value)
			}
			if !strings.Contains(err.Error(), EnvPrefix+tc.variable) {
				t.Errorf("the message does not name the failing setting:\n%v", err)
			}
		})
	}
}

func TestValidValuesAreAccepted(t *testing.T) {
	cfg, err := loadWith(t, map[string]string{
		"ENVIRONMENT":           "benchmark",
		"HTTP_PORT":             "9999",
		"LOG_LEVEL":             "warn",
		"SHUTDOWN_GRACE_PERIOD": "45s",
		"KAFKA_BROKERS":         "a:9092, b:9092 ,c:9092",
		"HEALTH_CHECK_TIMEOUT":  "5s",
		"HEALTH_CACHE_TTL":      "2s",
	})
	if err != nil {
		t.Fatalf("valid configuration was rejected: %v", err)
	}
	if len(cfg.KafkaBrokers) != 3 {
		t.Errorf("brokers = %v, want 3 entries with whitespace trimmed", cfg.KafkaBrokers)
	}
	if cfg.KafkaBrokers[1] != "b:9092" {
		t.Errorf("broker[1] = %q, want %q", cfg.KafkaBrokers[1], "b:9092")
	}
}
