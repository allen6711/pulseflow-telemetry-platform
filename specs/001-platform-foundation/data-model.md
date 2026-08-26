# Phase 1 Data Model: Platform Foundation & Local Development Environment

**Feature**: `001-platform-foundation` | **Date**: 2026-08-26

This feature persists nothing. The entities below are in-memory structures that define the shape of
this feature's contracts — configuration, health results, and log records. Field-level wire formats
live in [contracts/](./contracts/); this document defines meaning, validation, and state.

---

## 1. Service Configuration

Loaded once from environment variables at startup, validated as a whole, then immutable for the
process lifetime. Source of truth for field names and defaults:
[contracts/configuration.md](./contracts/configuration.md).

**Fields** (grouped by concern):

| Group | Field | Type | Default | Validation |
| --- | --- | --- | --- | --- |
| Identity | `ServiceName` | string | `pulseflow-api` / `pulseflow-worker` | non-empty |
| Identity | `Version` | string | `dev` | non-empty |
| Identity | `Environment` | string | `local` | one of `local`, `ci`, `benchmark` |
| Server | `HTTPPort` | int | `8080` (api) / `8081` (worker) | 1–65535 |
| Server | `ShutdownGracePeriod` | duration | `30s` | > 0, ≤ `5m` |
| Logging | `LogLevel` | string | `info` | one of `debug`, `info`, `warn`, `error` |
| Kafka | `KafkaBrokers` | []string | `localhost:9092` | ≥ 1 entry, each `host:port` |
| ClickHouse | `ClickHouseAddr` | string | `localhost:9000` | `host:port` |
| ClickHouse | `ClickHouseDatabase` | string | `pulseflow` | non-empty |
| ClickHouse | `ClickHouseUser` | string | `default` | non-empty |
| ClickHouse | `ClickHousePassword` | string (**sensitive**) | `""` | none |
| Redis | `RedisAddr` | string | `localhost:6379` | `host:port` |
| Redis | `RedisPassword` | string (**sensitive**) | `""` | none |
| Health | `HealthCheckTimeout` | duration | `2s` | > 0, ≤ `30s` |
| Health | `HealthCacheTTL` | duration | `1s` | ≥ 0, < `HealthCheckTimeout` |

**Validation rules**

- Validation collects **every** failure before returning, never short-circuits on the first
  (FR-016, SC-005). The returned error renders one line per failing field.
- Each failure line states three things: the environment variable name, the value received, and the
  permitted format or range. Example:
  `PULSEFLOW_HTTP_PORT: got "http", must be an integer between 1 and 65535`.
- Cross-field rule: `HealthCacheTTL` must be strictly less than `HealthCheckTimeout`, otherwise a
  cached failure outlives the check that produced it and FR-011's recovery window becomes
  unbounded.
- Fields marked **sensitive** are masked as `***` by `Config.String()` and never appear in logs
  (FR-018, FR-030).

**State**: `unvalidated → validated` (terminal) or `unvalidated → invalid` (terminal, process
exits non-zero). There is no reload path — configuration changes require a restart (FR-015,
FR-017).

---

## 2. Dependency Health Status

The result of checking one external dependency. Produced by a `Checker`; never persisted.

| Field | Type | Description |
| --- | --- | --- |
| `Name` | string | One of the fixed values `kafka`, `clickhouse`, `redis`. Bounded set — used as a metric label (Principle I) |
| `Healthy` | bool | Whether the dependency answered its protocol-level probe |
| `Duration` | duration | Wall time the check took, including timeout |
| `Reason` | string | Empty when healthy. When unhealthy, a classified reason, never a raw driver error |
| `CheckedAt` | timestamp | When the check completed |

**Reason classification** (bounded set, so it is safe to surface and to count):

| Reason | Meaning |
| --- | --- |
| `timeout` | The check exceeded `HealthCheckTimeout` |
| `connection_refused` | No listener, or the connection was actively refused |
| `auth_failed` | Reached the service but credentials were rejected |
| `protocol_error` | Connected, but the service did not answer its own protocol correctly |
| `unknown` | Anything not matching the above |

`Reason` deliberately carries a classification rather than the underlying error text, because the
readiness response is served over HTTP and driver errors routinely embed hostnames, ports, and
occasionally credentials (FR-012). The full error is logged instead, under `dependency_check_failed`.

**State**: each check produces one immutable status. There is no transition; recovery is expressed
by a subsequent check producing a different value.

---

## 3. Readiness Result

The aggregate of all dependency health statuses for one readiness evaluation, plus the process's own
lifecycle state. Cached for `HealthCacheTTL` and shared across concurrent requests via
`singleflight` (R-006).

| Field | Type | Description |
| --- | --- | --- |
| `Status` | enum | `ready` or `not_ready` |
| `Dependencies` | []DependencyHealthStatus | One entry per dependency, **always all three**, healthy or not (FR-008) |
| `EvaluatedAt` | timestamp | When this aggregate was produced; drives cache expiry |
| `ShuttingDown` | bool | True once a termination signal has been received |

**Aggregation rules**

- `Status` is `ready` only when `ShuttingDown` is false **and** every entry in `Dependencies` is
  healthy.
- `ShuttingDown` overrides dependency state entirely: once shutdown begins the result is
  `not_ready` immediately and unconditionally, without waiting for the cache to expire and without
  running dependency checks (FR-020).
- All dependency checks run concurrently; the aggregate's latency is the slowest check, not their
  sum.
- A cached result older than `HealthCacheTTL` triggers exactly one re-evaluation regardless of how
  many requests arrive simultaneously (FR-010).

**Relationship to liveness**: the liveness result is deliberately *not* modelled here. It is a
constant success response independent of every field above (FR-006). Keeping it structurally
separate is what prevents the two probes from converging by accident during later refactors.

**State transitions**:

```text
                   all deps healthy
  not_ready ──────────────────────────▶ ready
      ▲                                   │
      │        any dep unhealthy          │
      └───────────────────────────────────┘
      ▲
      │  termination signal received
      │  (one-way; never returns to ready)
  ready / not_ready ──▶ not_ready (ShuttingDown = true)
```

---

## 4. Log Record

One structured event, emitted as a single line of JSON to stdout. Field names and types are fixed by
[contracts/log-record.md](./contracts/log-record.md) and are a contract that later features consume
rather than redefine.

| Field | Type | Always present | Description |
| --- | --- | --- | --- |
| `time` | RFC3339 with milliseconds | yes | Event timestamp |
| `level` | string | yes | `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `msg` | string | yes | Short stable event name, e.g. `dependency_check_failed` |
| `service` | string | yes | From `ServiceName` |
| `version` | string | yes | From `Version` |
| `trace_id` | string (32 lowercase hex) | yes | Correlation identifier; all-zero when no correlated context exists |
| `error_class` | string | on error only | Same bounded classification vocabulary as `Reason` above |
| `error` | string | on error only | Full underlying error text — safe here, unlike in HTTP responses |
| *(arbitrary)* | any | no | Per-event context attributes |

**Rules**

- `msg` is a stable event name, not a formatted sentence. Variable data goes into attributes, so
  that logs stay groupable and searchable as later features add volume.
- `trace_id` is 16 bytes rendered as 32 lowercase hex characters — the W3C Trace Context trace ID
  format — so that F08 can source it from an OpenTelemetry span context without altering this
  contract (R-008).
- Records below the configured `LogLevel` are never emitted (FR-028).
- Sensitive configuration values never appear in any field (FR-030).

---

## 5. Correlation Context

Not a serialized entity — the mechanism carrying `trace_id` through a request's lifetime.

- Stored in `context.Context` under an unexported key, so no package outside `internal/logging` can
  overwrite it.
- Populated by HTTP middleware: if the request carries a `traceparent` header, its trace ID is
  adopted; otherwise a new random 16-byte ID is generated (FR-027).
- Read by the context-aware `slog.Handler` on every record, so handlers never pass `trace_id`
  manually (FR-026).
- When no correlation context is present — startup and shutdown logs, for instance — `trace_id` is
  the all-zero trace ID rather than an omitted field, keeping the record shape uniform for parsers
  (FR-025).

---

## Entity relationships

```text
Service Configuration ──drives──▶ Health Check Timeout, Cache TTL, Grace Period, Log Level
                      ──masks───▶ sensitive fields excluded from Log Record

Dependency Health Status ×3 ──aggregated into──▶ Readiness Result
                            ──failure emits───▶ Log Record (error_class)
                            ──observed as────▶ pulseflow_dependency_up{dependency}

Correlation Context ──injects trace_id into──▶ Log Record
```
