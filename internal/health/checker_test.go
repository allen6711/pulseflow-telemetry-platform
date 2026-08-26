package health

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// stubChecker is a Checker with a scripted outcome.
type stubChecker struct {
	name  string
	err   error
	delay time.Duration
	calls int
}

func (s *stubChecker) Name() string { return s.name }

func (s *stubChecker) Check(ctx context.Context) error {
	s.calls++
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.err
}

func TestClassifyMapsErrorsOntoTheBoundedVocabulary(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want Reason
	}{
		{"context deadline", context.DeadlineExceeded, ReasonTimeout},
		{"io deadline", os.ErrDeadlineExceeded, ReasonTimeout},
		{"wrapped deadline", fmt.Errorf("clickhouse ping: %w", context.DeadlineExceeded), ReasonTimeout},
		{"connection refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, ReasonConnectionRefused},
		{"wrapped refusal", fmt.Errorf("redis ping: %w", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}), ReasonConnectionRefused},
		{"redis noauth", errors.New("NOAUTH Authentication required"), ReasonAuthFailed},
		{"clickhouse auth", errors.New("code: 516, message: default: Authentication failed"), ReasonAuthFailed},
		{"other net error", &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}, ReasonUnknown},
		{"protocol level", errors.New("unexpected packet 42 from server"), ReasonProtocolError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.err); got != tc.want {
				t.Errorf("Classify(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestClassifyReturnsEmptyForSuccess(t *testing.T) {
	if got := Classify(nil); got != "" {
		t.Errorf("Classify(nil) = %q, want empty", got)
	}
}

// A driver error can embed a host, a port, or a credential, and the readiness
// response is reachable over HTTP. The reason must therefore be a
// classification, never the underlying text (FR-012).
func TestStatusReasonNeverCarriesRawDriverText(t *testing.T) {
	secret := "super-secret-password"
	checker := &stubChecker{
		name: DependencyClickHouse,
		err:  fmt.Errorf("dial tcp 10.0.0.5:9000: auth for user default with password %s failed", secret),
	}

	st := run(context.Background(), checker, time.Second)

	if st.Healthy {
		t.Fatal("expected an unhealthy status")
	}
	if strings.Contains(string(st.Reason), secret) || strings.Contains(string(st.Reason), "10.0.0.5") {
		t.Errorf("reason leaked driver detail: %q", st.Reason)
	}
	switch st.Reason {
	case ReasonTimeout, ReasonConnectionRefused, ReasonAuthFailed, ReasonProtocolError, ReasonUnknown:
	default:
		t.Errorf("reason %q is outside the bounded vocabulary", st.Reason)
	}
	// The full error is still available for the logs.
	if st.Err == nil {
		t.Error("Err must retain the underlying error for logging")
	}
}

func TestRunTreatsTimeoutAsUnhealthy(t *testing.T) {
	checker := &stubChecker{name: DependencyKafka, delay: 500 * time.Millisecond}

	st := run(context.Background(), checker, 50*time.Millisecond)

	if st.Healthy {
		t.Error("a check that exceeded its timeout must be unhealthy")
	}
	if st.Reason != ReasonTimeout {
		t.Errorf("reason = %q, want %q", st.Reason, ReasonTimeout)
	}
}
