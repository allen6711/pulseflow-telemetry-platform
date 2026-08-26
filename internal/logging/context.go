// Package logging provides PulseFlow's structured logging: a JSON handler that
// emits the field set fixed by contracts/log-record.md, and the correlation
// identifier that ties one request's records together.
//
// The correlation identifier is 16 bytes rendered as 32 lowercase hex
// characters -- the W3C Trace Context trace ID format. That choice is
// deliberate: when OpenTelemetry arrives in F08 the value can be sourced from a
// span context without changing this contract, so every log query and dashboard
// built in the meantime keeps working.
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// TraceIDLen is the length of a trace identifier in bytes.
const TraceIDLen = 16

// TraceID is a W3C Trace Context trace identifier.
type TraceID [TraceIDLen]byte

// zeroTraceID is emitted when a record has no correlation context: startup,
// shutdown, and background work. Emitting the all-zero value rather than
// omitting the field keeps every record the same shape for parsers.
var zeroTraceID TraceID

// String renders the identifier as 32 lowercase hex characters.
func (t TraceID) String() string { return hex.EncodeToString(t[:]) }

// IsZero reports whether this is the all-zero identifier.
func (t TraceID) IsZero() bool { return t == zeroTraceID }

// NewTraceID returns a random trace identifier.
func NewTraceID() TraceID {
	var id TraceID
	// crypto/rand.Read never fails on the platforms this runs on; since Go 1.24
	// it panics rather than returning an error.
	_, _ = rand.Read(id[:])
	return id
}

// ParseTraceID decodes 32 hex characters into a trace identifier. It reports
// false for anything malformed or all-zero, since an all-zero inbound trace ID
// carries no correlation and should be replaced rather than adopted.
func ParseTraceID(s string) (TraceID, bool) {
	var id TraceID
	if len(s) != TraceIDLen*2 {
		return zeroTraceID, false
	}
	if _, err := hex.Decode(id[:], []byte(s)); err != nil {
		return zeroTraceID, false
	}
	if id.IsZero() {
		return zeroTraceID, false
	}
	return id, true
}

// contextKey is unexported so no package outside this one can overwrite the
// correlation identifier a request is carrying.
type contextKey struct{}

// ContextWithTraceID returns a context carrying the given trace identifier.
func ContextWithTraceID(ctx context.Context, id TraceID) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// TraceIDFromContext returns the trace identifier the context carries, or the
// all-zero identifier when there is none.
func TraceIDFromContext(ctx context.Context) TraceID {
	if id, ok := ctx.Value(contextKey{}).(TraceID); ok {
		return id
	}
	return zeroTraceID
}

// TraceparentHeader is the W3C Trace Context header carrying the trace ID.
const TraceparentHeader = "traceparent"

// TraceIDFromTraceparent extracts the trace ID from a W3C traceparent header.
//
// The header is version-dashed-traceid-dashed-spanid-dashed-flags. Only the
// trace ID is taken: this project does not participate in span propagation
// until F08, and adopting the caller's trace ID is what makes a request's logs
// correlate end to end today.
func TraceIDFromTraceparent(header string) (TraceID, bool) {
	parts := strings.Split(strings.TrimSpace(header), "-")
	if len(parts) < 4 {
		return zeroTraceID, false
	}
	// Version 00 is the only one defined; later versions keep this field order.
	if len(parts[0]) != 2 {
		return zeroTraceID, false
	}
	return ParseTraceID(parts[1])
}

// TraceIDForRequest adopts an inbound traceparent's trace ID when present and
// well-formed, and generates a fresh one otherwise (FR-027).
func TraceIDForRequest(header string) TraceID {
	if id, ok := TraceIDFromTraceparent(header); ok {
		return id
	}
	return NewTraceID()
}
