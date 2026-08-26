package health

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

// Dependency names. This set is fixed and small, which is what makes it safe to
// use as a metric label under Constitution Principle I.
const (
	DependencyKafka      = "kafka"
	DependencyClickHouse = "clickhouse"
	DependencyRedis      = "redis"
)

// Reason classifies why a dependency check failed.
//
// The vocabulary is deliberately bounded and shared with the log contract's
// error_class field, so failures can be counted and correlated between logs and
// metrics rather than parsed out of free text.
type Reason string

const (
	ReasonTimeout           Reason = "timeout"
	ReasonConnectionRefused Reason = "connection_refused"
	ReasonAuthFailed        Reason = "auth_failed"
	ReasonProtocolError     Reason = "protocol_error"
	ReasonUnknown           Reason = "unknown"
)

// Status is the result of checking one dependency.
//
// Err carries the full underlying error for the logs. It is unexported to JSON
// on purpose: the readiness response is served over HTTP, and driver errors
// routinely embed hosts, ports, and occasionally credentials (FR-012).
type Status struct {
	Name       string    `json:"name"`
	Healthy    bool      `json:"healthy"`
	DurationMS int64     `json:"duration_ms"`
	Reason     Reason    `json:"reason,omitempty"`
	CheckedAt  time.Time `json:"-"`
	Err        error     `json:"-"`
}

// Checker probes one dependency.
//
// Implementations construct their client lazily so that an unreachable
// dependency never prevents the process from starting (FR-037).
type Checker interface {
	// Name returns the dependency's fixed name.
	Name() string
	// Check returns nil when the dependency answered its own protocol.
	Check(ctx context.Context) error
}

// run executes one check and turns the outcome into a Status.
func run(ctx context.Context, c Checker, timeout time.Duration) Status {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	err := c.Check(ctx)
	elapsed := time.Since(start)

	st := Status{
		Name:       c.Name(),
		Healthy:    err == nil,
		DurationMS: elapsed.Milliseconds(),
		CheckedAt:  time.Now().UTC(),
	}
	if err != nil {
		st.Reason = Classify(err)
		st.Err = err
	}
	return st
}

// Classify maps an error onto the bounded reason vocabulary.
func Classify(err error) Reason {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return ReasonTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ReasonTimeout
	}

	if errors.Is(err, syscall.ECONNREFUSED) {
		return ReasonConnectionRefused
	}

	if looksLikeAuthFailure(err.Error()) {
		return ReasonAuthFailed
	}

	// A network-layer error that is neither a timeout nor a refusal: the
	// connection itself did not work, so we cannot say the service misbehaved.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return ReasonUnknown
	}

	// Anything else means we reached the service and it answered incorrectly.
	return ReasonProtocolError
}

// authMarkers are substrings the three clients use for credential rejection.
// Matching on text is a heuristic, but the alternative is three driver-specific
// error taxonomies leaking into this package.
var authMarkers = []string{
	"authentication",
	"authentication failed",
	"access denied",
	"password",
	"unauthorized",
	"noauth",
	"wrongpass",
	"sasl",
}

func looksLikeAuthFailure(msg string) bool {
	msg = strings.ToLower(msg)
	for _, marker := range authMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
