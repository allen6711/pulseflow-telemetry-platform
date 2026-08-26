package logging

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// FR-030 and the "Prohibited content" rule in contracts/log-record.md: a
// credential embedded in driver error text must not reach the log output, while
// the rest of the message stays useful for diagnosis.
func TestSanitizerStripsCredentialsAndKeepsDiagnostics(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		secret string
		keep   []string
	}{
		{
			name:   "dsn userinfo",
			in:     "dial clickhouse://admin:s3cr3t-pw@clickhouse:9000/pulseflow: connection refused",
			secret: "s3cr3t-pw",
			keep:   []string{"clickhouse:9000", "connection refused", "admin"},
		},
		{
			name:   "password key value",
			in:     "auth failed for user=default password=hunter2 on host 10.1.2.3",
			secret: "hunter2",
			keep:   []string{"user=default", "10.1.2.3", "auth failed"},
		},
		{
			name:   "quoted token",
			in:     `sasl handshake rejected: token="abc.def.ghi" expired`,
			secret: "abc.def.ghi",
			keep:   []string{"sasl handshake rejected", "expired"},
		},
		{
			name:   "api key with dash",
			in:     "request denied: api-key: AKIA1234567890 not recognised",
			secret: "AKIA1234567890",
			keep:   []string{"request denied", "not recognised"},
		},
		{
			name:   "redis wrongpass",
			in:     "WRONGPASS invalid username-password pair; password=letmein",
			secret: "letmein",
			keep:   []string{"WRONGPASS"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeString(tc.in)

			if strings.Contains(got, tc.secret) {
				t.Errorf("the secret survived sanitizing:\n  in:  %s\n  out: %s", tc.in, got)
			}
			if !strings.Contains(got, Redacted) {
				t.Errorf("nothing was redacted, so the pattern did not match:\n  out: %s", got)
			}
			for _, keep := range tc.keep {
				if !strings.Contains(got, keep) {
					t.Errorf("sanitizing removed diagnostic detail %q:\n  out: %s", keep, got)
				}
			}
		})
	}
}

func TestSanitizerLeavesCleanTextAlone(t *testing.T) {
	clean := "dial tcp 127.0.0.1:9000: connect: connection refused"

	if got := SanitizeString(clean); got != clean {
		t.Errorf("clean text was altered:\n  in:  %s\n  out: %s", clean, got)
	}
}

func TestSanitizeErrorHandlesNil(t *testing.T) {
	if got := SanitizeError(nil); got != "" {
		t.Errorf("SanitizeError(nil) = %q, want empty", got)
	}
}

func TestErrorAttributeIsSanitized(t *testing.T) {
	err := fmt.Errorf("clickhouse ping: %w", errors.New("auth failed password=hunter2"))

	attr := Error(err)

	if attr.Key != "error" {
		t.Errorf("attribute key = %q, want error", attr.Key)
	}
	if strings.Contains(attr.Value.String(), "hunter2") {
		t.Errorf("the error attribute leaked a secret: %s", attr.Value)
	}
}
