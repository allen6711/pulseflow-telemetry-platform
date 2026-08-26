package config

import (
	"strings"
	"testing"
)

// FR-016: every message names the setting, quotes what it received, and states
// what would be acceptable -- enough for a reader to fix it without opening the
// source.
func TestMessagesNameTheSettingValueAndExpectation(t *testing.T) {
	cases := []struct {
		variable  string
		value     string
		wantParts []string
	}{
		{"HTTP_PORT", "http", []string{EnvPrefix + "HTTP_PORT", `"http"`, "integer"}},
		{"HTTP_PORT", "70000", []string{EnvPrefix + "HTTP_PORT", `"70000"`, "between 1 and 65535"}},
		{"LOG_LEVEL", "verbose", []string{EnvPrefix + "LOG_LEVEL", `"verbose"`, "debug, info, warn, error"}},
		{"ENVIRONMENT", "prod", []string{EnvPrefix + "ENVIRONMENT", `"prod"`, "local, ci, benchmark"}},
		{"KAFKA_BROKERS", "localhost", []string{EnvPrefix + "KAFKA_BROKERS", `"localhost"`, "host:port"}},
		{"SHUTDOWN_GRACE_PERIOD", "10m", []string{EnvPrefix + "SHUTDOWN_GRACE_PERIOD", "at most 5m0s"}},
	}

	for _, tc := range cases {
		t.Run(tc.variable+"="+tc.value, func(t *testing.T) {
			t.Setenv(EnvPrefix+tc.variable, tc.value)
			_, err := Load(APIDefaults)
			if err == nil {
				t.Fatal("expected a failure")
			}
			msg := err.Error()
			for _, part := range tc.wantParts {
				if !strings.Contains(msg, part) {
					t.Errorf("message is missing %q:\n%s", part, msg)
				}
			}
		})
	}
}

func TestErrorCountsAreRenderedCorrectly(t *testing.T) {
	single := &ValidationError{Problems: []Problem{{Variable: "A", Got: "x", Want: "must be y"}}}
	if !strings.Contains(single.Error(), "(1 error)") {
		t.Errorf("single problem renders as: %s", single.Error())
	}

	double := &ValidationError{Problems: []Problem{
		{Variable: "A", Got: "x", Want: "must be y"},
		{Variable: "B", Got: "z", Want: "must be w"},
	}}
	if !strings.Contains(double.Error(), "(2 errors)") {
		t.Errorf("two problems render as: %s", double.Error())
	}
	if lines := strings.Count(double.Error(), "\n"); lines != 2 {
		t.Errorf("expected one line per problem, got %d lines:\n%s", lines, double.Error())
	}
}
