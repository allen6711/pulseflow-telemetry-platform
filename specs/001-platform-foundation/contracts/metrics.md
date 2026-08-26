# Contract: Metrics

**Feature**: `001-platform-foundation` | **Consumers**: F02–F09, and F08 in particular

Both services expose `/metrics` in Prometheus text exposition format from a non-global
`prometheus.Registry` owned by `internal/observability`. Later features register into that same
registry; none creates its own or uses `prometheus.DefaultRegisterer`.

## Naming rules

1. Every metric name starts with `pulseflow_`, except the Go runtime and process collectors, which
   keep their standard `go_` and `process_` names.
2. Units are suffixes and always base units: `_seconds`, `_bytes`, `_total` for counters.
3. Metric names are a stable contract. Renaming one breaks dashboards and saved queries and requires
   updating this file in the same change.

## Label cardinality rules

Bounded cardinality is a constitutional requirement, not a style preference (Principle I). A label
value must come from a set the code controls, never from request data.

- **Never a label**: `event_id`, `trace_id`, raw telemetry `tags` values, full URL paths, user
  identifiers, error message text.
- **Acceptable as a label**: registered route patterns, HTTP methods, status codes, the fixed
  dependency names, the bounded error classification vocabulary.

The `route` label uses the `ServeMux` pattern (`/v1/metrics/{service}/{metric}`), never the
concrete request path — otherwise F06's analytics endpoint would mint a new time series per service
and metric combination queried.

## Metrics registered by this feature

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `pulseflow_build_info` | Gauge (always 1) | `version`, `commit`, `go_version` | Build identification for the running binary |
| `pulseflow_http_requests_total` | Counter | `route`, `method`, `code` | HTTP requests served, by outcome |
| `pulseflow_http_request_duration_seconds` | Histogram | `route`, `method` | HTTP request latency |
| `pulseflow_dependency_up` | Gauge (0 or 1) | `dependency` | Result of the most recent readiness check per dependency |
| `pulseflow_dependency_check_duration_seconds` | Histogram | `dependency` | Duration of readiness checks, including timeouts |
| `go_*`, `process_*` | various | — | Standard Go runtime and process collectors |

`dependency` takes exactly one of `kafka`, `clickhouse`, `redis`.

## Deliberately not registered here

Business metrics belong to the features that own the paths they measure, and F08 owns unifying them.
This feature does not define ingestion rate, consumer lag, processing failures, dead-letter counts,
storage write latency, or cache hit ratio. Each later feature registers what its own paths need,
under these naming and cardinality rules; F08 then fills gaps and audits the whole catalog against
README §6.

## Scrape configuration

`deployments/docker/prometheus.yml` scrapes both services at a 5-second interval in the local stack.
Both targets are declared statically — service discovery arrives with F11.
