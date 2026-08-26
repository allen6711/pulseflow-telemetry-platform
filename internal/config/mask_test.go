package config

import (
	"log/slog"
	"strings"
	"testing"
)

const secret = "hunter2"

func configWithSecrets(t *testing.T) Config {
	t.Helper()
	t.Setenv(EnvPrefix+"CLICKHOUSE_PASSWORD", secret)
	t.Setenv(EnvPrefix+"REDIS_PASSWORD", secret+"-redis")

	cfg, err := Load(APIDefaults)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	return cfg
}

func TestStringMasksSensitiveValues(t *testing.T) {
	cfg := configWithSecrets(t)

	rendered := cfg.String()

	if strings.Contains(rendered, secret) {
		t.Errorf("a secret survived masking:\n%s", rendered)
	}
	if !strings.Contains(rendered, "clickhouse_password="+Masked) {
		t.Errorf("clickhouse password is not masked:\n%s", rendered)
	}
	if !strings.Contains(rendered, "redis_password="+Masked) {
		t.Errorf("redis password is not masked:\n%s", rendered)
	}
	// Non-sensitive values must stay readable, or the rendering is useless.
	if !strings.Contains(rendered, "clickhouse_user=default") {
		t.Errorf("non-sensitive values should not be masked:\n%s", rendered)
	}
}

func TestEmptySecretRendersAsEmptyNotMasked(t *testing.T) {
	cfg, err := Load(APIDefaults)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if strings.Contains(cfg.String(), "clickhouse_password="+Masked) {
		t.Error(`an unset password should render empty, so "not configured" is distinguishable from "withheld"`)
	}
}

// Passing a Config to slog must not bypass masking.
func TestLoggingAConfigDoesNotLeak(t *testing.T) {
	cfg := configWithSecrets(t)

	var sb strings.Builder
	slog.New(slog.NewJSONHandler(&sb, nil)).Info("service_started", slog.Any("config", cfg))

	if strings.Contains(sb.String(), secret) {
		t.Errorf("a secret reached the log output:\n%s", sb.String())
	}
}
