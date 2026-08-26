# Implementation Plan: Platform Foundation & Local Development Environment

**Branch**: `001-platform-foundation` | **Date**: 2026-08-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-platform-foundation/spec.md`

## Summary

Deliver the platform shell that every later PulseFlow feature builds on: two Go binaries (`cmd/api`,
`cmd/worker`) that start, validate their configuration, expose correctly-separated liveness and
readiness probes, log in a structured format carrying a trace-compatible correlation ID, expose a
metrics endpoint, and shut down gracefully — plus a pinned Docker Compose stack (Kafka, ClickHouse,
Redis, Prometheus) with real protocol-level healthchecks, a Makefile that is the single entry point
for build/test/lint/compose, and a CI workflow that runs exactly what the Makefile runs.

The technical approach centers on three decisions from [research.md](./research.md): readiness is a
concurrent, timeout-bounded, `singleflight`-collapsed and briefly-cached aggregation of
protocol-level dependency pings (R-006); configuration is hand-written so that it can accumulate all
validation errors and own its message text (R-007); and the correlation ID is a W3C-shaped trace ID
from day one so F08 can introduce OpenTelemetry without changing the log contract (R-008). No
business logic ships in this feature.

## Technical Context

**Language/Version**: Go 1.25 declared in `go.mod`; developed against the locally installed Go
1.26.5 toolchain (R-001)

**Primary Dependencies**: `net/http` stdlib routing (R-002) · `twmb/franz-go` + `kadm` for Kafka
(R-003) · `ClickHouse/clickhouse-go/v2` and `redis/go-redis/v9` (R-004) ·
`prometheus/client_golang` (R-010) · `golang.org/x/sync/singleflight` (R-006) · `log/slog` stdlib
(R-008). Tool dependency: `golangci-lint` pinned via the `go.mod` `tool` directive (R-011)

**Storage**: None in this feature. ClickHouse must start and accept connections, but no schema,
migration, or table is created here — that is F03

**Testing**: Standard library `testing`, no assertion framework. Unit tests run by default;
integration tests carry a `//go:build integration` tag and run against the compose stack (R-012)

**Target Platform**: Linux containers for deployment; macOS (arm64) and Linux for development.
Requires Docker Engine 25+ and Compose v2+ (observed locally: Docker 29.3.1, Compose v5.1.1)

**Project Type**: Multi-binary backend service — two Go executables sharing internal packages,
orchestrated locally by Docker Compose

**Performance Goals**: None claimed in this feature. The only latency constraint is operational: a
readiness probe must respond within its configured timeout budget (~2s worst case) rather than
blocking. Throughput targets belong to F09, and per constitution Principle III no number here is
presented as a measured result

**Constraints**: Readiness dependency checks bounded at 2s each and collapsed to at most one check
round per 1s window (FR-009, FR-010) · shutdown grace period 30s default (FR-021) · metric label
cardinality bounded, no `event_id` or user-controlled strings as labels (Principle I) · all
container images pinned to explicit patch tags (FR-003)

**Scale/Scope**: 6 user stories, 36 functional requirements, 10 success criteria. Roughly 6 internal
packages, 2 binaries, 1 compose file, 1 Makefile, 1 CI workflow. No persistent data, no external
consumers yet

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` v1.0.1.

### Principle I — Observability First

**Gate**: every new processing path maps to a named metric and a named log event.

| Processing path introduced | Metric | Log event |
| --- | --- | --- |
| HTTP request handling (health + metrics routes) | `pulseflow_http_requests_total{route,method,code}`, `pulseflow_http_request_duration_seconds{route,method}` | `http_request` (debug), `http_request_failed` (error) |
| Dependency readiness check | `pulseflow_dependency_up{dependency}`, `pulseflow_dependency_check_duration_seconds{dependency}` | `dependency_check_failed` |
| Service startup | `pulseflow_build_info{version,commit,go_version}` | `service_started`, `config_validation_failed` |
| Graceful shutdown | — (bounded lifecycle event, covered by logs) | `shutdown_started`, `shutdown_complete`, `shutdown_timeout` |

Label cardinality: `route` uses the registered `ServeMux` pattern, never the raw path;
`dependency` is one of three fixed values. Both bounded. **PASS** (R-010)

### Principle II — Layered Testing (NON-NEGOTIABLE)

**Gate**: the task list contains test tasks for every layer the feature touches.

- Pure logic → unit tests: configuration parsing and validation error accumulation, log record
  shape and `trace_id` injection, readiness result aggregation across mixed dependency states,
  correlation ID generation and `traceparent` adoption, shutdown state machine transitions.
- External boundary → integration tests (`//go:build integration`, against compose): each of the
  three dependency probes against a live service, each probe against a stopped service, and a
  service started while the whole stack is down asserting it starts and reports not-ready.
- End-to-end → out of scope for this feature by design; the full-stack chain does not exist until
  F10.

No mock is substituted for a real service at any boundary. **PASS** (R-012)

### Principle III — Reproducible Measurement

**Gate**: every number in the spec or plan traces to a committed script and a raw output file.

This feature makes no performance claim. The three success criteria carrying numbers — SC-001
(10-minute cold start), SC-009 (5-minute CI feedback), SC-010 (5 consecutive start/stop cycles) —
are repeatable procedures, and each gets a committed script (`scripts/verify-cold-start.sh`,
`scripts/verify-restart-idempotence.sh`; CI duration is read from the workflow run). Their observed
values are recorded when measured and are not asserted anywhere as achieved outcomes today.
**PASS**

### Principle IV — At-Least-Once Delivery & Application-Level Idempotency

**Gate**: every write to an external system states its failure classification, offset behavior, and
idempotency basis.

This feature performs no event processing and no write to any external system. No Kafka offsets are
committed, no records are persisted, and no endpoint returns `202`. **NOT APPLICABLE** — the
obligation begins at F02 (ingestion acknowledgement) and F04 (offset commit ordering).

### Principle V — Simplicity & Scope Discipline

**Gate**: every new component, package, or abstraction names the concrete problem it solves today.

| Addition | Problem it solves today |
| --- | --- |
| `franz-go`, `clickhouse-go/v2`, `go-redis` | FR-007's readiness probe requires a protocol-level answer from each dependency; a TCP dial cannot distinguish a listening socket from a working broker (R-006) |
| `prometheus/client_golang` | FR-031/FR-033 require a scrapeable `/metrics` endpoint |
| `x/sync/singleflight` | FR-010's requirement that high-frequency probes not amplify load on dependencies |
| `internal/config` (hand-written) | FR-016's error message contract, which struct-tag libraries cannot express (R-007) |
| `internal/observability` | Shared metric registry so F02–F08 register into one place rather than each creating a global |

Explicitly **not** added: an HTTP router (stdlib suffices, R-002), a configuration library (R-007),
an assertion framework (R-012), a logging library (R-008), testcontainers (R-012). Two binaries, not
a microservice split. **PASS**

### Post-Phase-1 re-check

Re-evaluated after producing [data-model.md](./data-model.md),
[contracts/](./contracts/), and [quickstart.md](./quickstart.md): the design introduced no component,
dependency, or abstraction beyond those listed above, and no gate result changed. All applicable
gates still **PASS**; Principle IV remains not applicable. **Complexity Tracking is empty.**

## Project Structure

### Documentation (this feature)

```text
specs/001-platform-foundation/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — 14 technical decisions
├── data-model.md        # Phase 1 output — configuration, health, and log entities
├── quickstart.md        # Phase 1 output — runnable validation guide
├── contracts/           # Phase 1 output
│   ├── health-api.yaml       # OpenAPI 3.1 for /v1/health/live, /v1/health/ready
│   ├── configuration.md      # Environment variable contract
│   ├── metrics.md            # Metric name and label contract
│   └── log-record.md         # Structured log field contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (passed)
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
├── api/
│   └── main.go                  # Wire config → logger → registry → deps → server → shutdown
└── worker/
    └── main.go                  # Same lifecycle; consumes nothing in this feature

internal/
├── config/
│   ├── config.go                # Typed settings, defaults, sensitive-field masking
│   ├── parse.go                 # Typed getters, multi-error accumulation
│   └── config_test.go
├── logging/
│   ├── logger.go                # slog JSON handler + service/version attributes
│   ├── context.go               # Correlation ID in context; traceparent adoption
│   ├── handler.go               # Context-aware handler injecting trace_id
│   └── logging_test.go
├── observability/
│   ├── registry.go              # Non-global prometheus.Registry, build_info
│   ├── httpmetrics.go           # Route-labelled request counter and histogram
│   └── observability_test.go
├── health/
│   ├── checker.go               # Checker interface; per-dependency implementations
│   ├── aggregate.go             # Concurrent fan-out, singleflight, min re-check interval
│   ├── handler.go               # /v1/health/live and /v1/health/ready handlers
│   └── health_test.go
├── httpserver/
│   ├── server.go                # ServeMux wiring, middleware chain, Shutdown
│   ├── middleware.go            # Correlation ID, logging, metrics, recovery
│   └── httpserver_test.go
└── lifecycle/
    ├── shutdown.go              # Signal handling, grace period, second-signal exit
    └── shutdown_test.go

deployments/
└── docker/
    ├── Dockerfile.api
    ├── Dockerfile.worker
    └── prometheus.yml           # Scrape config for api and worker

tests/
└── integration/
    ├── deps_test.go             # //go:build integration — probes against live stack
    └── startup_test.go          # //go:build integration — starts with stack down

scripts/
├── verify-cold-start.sh         # SC-001
└── verify-restart-idempotence.sh # SC-010

.github/workflows/
└── ci.yml                       # build · vet · lint · unit · integration

docker-compose.yml
Makefile
go.mod
go.sum
```

**Structure Decision**: The layout follows the repository layout already published in `README.md`,
with `internal/` split by capability rather than by architectural layer. Only the directories this
feature actually needs are created — `internal/telemetry`, `internal/kafka`, `internal/storage`, and
`internal/cache` from the README's tree are deliberately deferred to the features that own them
(F02, F03, F04, F07), since creating empty packages now would be organizational-only structure with
no behavior, which constitution Principle V rejects. `migrations/`, `deployments/k8s/`,
`benchmarks/`, and `tests/e2e/` are likewise created by F03, F11, F09, and F10 respectively.

Both binaries share every `internal/` package listed. The worker's `main.go` differs from the API's
only in which HTTP routes it registers (health and metrics only) and in having no request-serving
role — it exists in this feature to prove the shared lifecycle works identically in both processes,
which is what FR-013 and FR-019 require.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Every gate in the Constitution Check passes or is not applicable, and no addition
required a justification beyond the concrete present-day problem recorded in the Principle V table
above.
