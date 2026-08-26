# Contract: Structured Log Record

**Feature**: `001-platform-foundation` | **Consumers**: every later feature, and F08 in particular

One JSON object per line on stdout. Collection and retention are left to the container runtime.

## Field contract

| Field | Type | Presence | Notes |
| --- | --- | --- | --- |
| `time` | RFC3339 with milliseconds, UTC | always | |
| `level` | `DEBUG` \| `INFO` \| `WARN` \| `ERROR` | always | |
| `msg` | string | always | Stable event name, not a formatted sentence |
| `service` | string | always | From `PULSEFLOW_SERVICE_NAME` |
| `version` | string | always | From `PULSEFLOW_VERSION` |
| `trace_id` | 32 lowercase hex characters | always | All zeros when there is no correlated context |
| `error_class` | string | error records only | Bounded vocabulary, see below |
| `error` | string | error records only | Full underlying error text |
| *(other)* | any | optional | Per-event context attributes |

## The `trace_id` field

`trace_id` is 16 bytes rendered as 32 lowercase hex characters — the W3C Trace Context trace ID
format. This is chosen deliberately so that when F08 introduces OpenTelemetry, the value can be
sourced from `trace.SpanContextFromContext(ctx).TraceID()` and this contract does not change. Every
log query, filter, and dashboard built between now and F08 keeps working.

Population rules:

- An inbound HTTP request carrying a `traceparent` header adopts that header's trace ID.
- A request without one gets a freshly generated random 16-byte ID.
- The ID lives in `context.Context` and is read automatically by the log handler, so call sites
  never pass it explicitly.
- Records with no correlated context — startup, shutdown, background work — carry the all-zero trace
  ID rather than omitting the field, so every record has the same shape for parsers.

## The `msg` field

`msg` is a stable, greppable event name. Variable data belongs in attributes.

```json
{"time":"2026-08-26T09:15:31.902Z","level":"ERROR","msg":"dependency_check_failed",
 "service":"pulseflow-api","version":"0.1.0",
 "trace_id":"4bf92f3577b34da6a3ce929d0e0e4736",
 "dependency":"clickhouse","error_class":"timeout","duration_ms":2001,
 "error":"dial tcp 127.0.0.1:9000: i/o timeout"}
```

Not: `"msg": "ClickHouse check failed after 2001ms"` — that cannot be grouped or counted.

## Event names introduced by this feature

| Event | Level | Attributes |
| --- | --- | --- |
| `service_started` | INFO | `port`, `environment`, masked configuration summary |
| `config_validation_failed` | ERROR | `errors` (array of per-field messages) |
| `http_request` | DEBUG | `route`, `method`, `code`, `duration_ms` |
| `http_request_failed` | ERROR | `route`, `method`, `code`, `error_class`, `error` |
| `dependency_check_failed` | ERROR | `dependency`, `error_class`, `duration_ms`, `error` |
| `shutdown_started` | INFO | `signal`, `grace_period` |
| `shutdown_complete` | INFO | `duration_ms` |
| `shutdown_timeout` | ERROR | `grace_period`, `pending` |

## Error classification vocabulary

`error_class` shares the bounded vocabulary used by the readiness contract, so failures can be
counted and correlated between logs and metrics: `timeout`, `connection_refused`, `auth_failed`,
`protocol_error`, `unknown`. Later features extend this list here rather than inventing per-package
vocabularies.

## Prohibited content

Sensitive configuration values (`PULSEFLOW_CLICKHOUSE_PASSWORD`, `PULSEFLOW_REDIS_PASSWORD`, and any
credential added later) never appear in any field, including inside `error`. Where a driver error
might embed a credential, it is sanitized before being attached.

Note the asymmetry with the readiness HTTP response, which carries only `error_class` and never the
raw error: logs are an internal channel and may carry the full error text, while the HTTP response
is externally reachable and must not.
