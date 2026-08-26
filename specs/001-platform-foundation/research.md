# Phase 0 Research: Platform Foundation & Local Development Environment

**Feature**: `001-platform-foundation` | **Date**: 2026-08-26

This document resolves every technical unknown in the plan's Technical Context. Each decision
records what was chosen, why, and what was rejected. Decisions marked **(binding for later
features)** are inherited by F02–F11 and should not be re-litigated per feature.

Local toolchain observed while researching: Go 1.26.5 (darwin/arm64), Docker 29.3.1,
Docker Compose v5.1.1, `golangci-lint` not installed globally.

---

## R-001: Go language version

**Decision**: Target Go 1.25 in `go.mod` (`go 1.25`), develop on the locally installed 1.26.5.

**Rationale**: Pinning the language version one minor release behind the newest toolchain keeps CI
runners and contributor machines from being forced onto a release that may not yet be available in
every base image, while still giving access to everything this feature needs: the enhanced
`net/http.ServeMux` routing patterns (1.22+), `log/slog` (1.21+), and the `tool` directive in
`go.mod` (1.24+). Go's forward compatibility means the 1.26.5 toolchain builds a `go 1.25` module
without issue.

**Alternatives considered**:

- `go 1.26` — matches the local toolchain exactly, but narrows the set of usable CI images and
  contributor environments for no capability gain.
- `go 1.22` — maximum compatibility, but forfeits the `tool` directive, which is the mechanism this
  plan uses to pin lint tooling reproducibly (see R-011).

---

## R-002: HTTP routing **(binding for later features)**

**Decision**: Standard library `net/http` with `http.ServeMux` method-and-pattern routing. No
third-party router.

**Rationale**: Since Go 1.22 `ServeMux` supports `GET /v1/metrics/{service}/{metric}` style
patterns with method matching and path parameter extraction, which covers every endpoint in the
README's API summary including the F06 analytics routes. The README itself allows either
`net/http` or a lightweight router; constitution Principle V requires a new dependency to solve a
problem that exists today, and no such problem exists.

**Alternatives considered**:

- `chi` — good middleware ergonomics, but its main advantage over the modern `ServeMux` is
  middleware chaining, which is a handful of lines to write directly.
- `gin` / `echo` — full frameworks with their own context types; they would leak into every handler
  signature across all later features for no benefit at this scale.

---

## R-003: Kafka client library **(binding for later features)**

**Decision**: `github.com/twmb/franz-go` (plus `franz-go/pkg/kadm` for admin operations such as the
readiness probe's metadata request and, later, consumer lag).

**Rationale**: This choice is dominated by F04 and F05, not by F01. Those features require a
consumer group that rebalances correctly under replica changes, offsets committed strictly after
persistence, and observable consumer lag. franz-go is pure Go (no cgo, so container images stay
simple and cross-compilation works), gives explicit manual offset-commit control rather than
committing behind the caller's back, and ships `kadm` for lag and metadata queries that F08 needs.
Deciding it now avoids F02 picking a producer library that F04 then has to work around.

**Alternatives considered**:

- `segmentio/kafka-go` — the simplest API of the three, but its consumer group implementation has
  historically been the weakest part of the library, and F05's restart and rebalance drills are a
  headline deliverable of this project.
- `IBM/sarama` — mature and widely deployed, with solid consumer groups, but a substantially larger
  API surface and more configuration knobs to get wrong; its offset management defaults are easier
  to misuse in a way that silently violates constitution Principle IV.
- `confluentinc/confluent-kafka-go` — the reference implementation in terms of protocol coverage,
  but it is a cgo binding to librdkafka, which complicates container builds and static linking for
  no benefit here.

---

## R-004: ClickHouse and Redis clients **(binding for later features)**

**Decision**: `github.com/ClickHouse/clickhouse-go/v2` (native protocol, not HTTP) and
`github.com/redis/go-redis/v9`.

**Rationale**: Both are the vendor-maintained, de facto standard clients for their respective
systems. `clickhouse-go/v2` exposes native-protocol batch insert (`PrepareBatch`), which F03's
batch write strategy depends on and which the HTTP interface cannot match for throughput. `go-redis`
exposes the `Ping` used by this feature's readiness probe and the pipelining F07 will want.

**Alternatives considered**:

- ClickHouse over HTTP with `database/sql` — simpler to debug with curl, but gives up efficient
  batch insert, which is central to the throughput numbers this project exists to measure.
- `rueidis` for Redis — measurably faster under high concurrency, but F07's cache sits behind an
  analytics query path where ClickHouse dominates latency; the added API unfamiliarity is not
  repaid.

---

## R-005: Kafka container image and mode

**Decision**: `apache/kafka:4.1.0` running in KRaft mode (no ZooKeeper), single broker for local
development, with the topic partition count configurable so F04 can scale consumers.

**Rationale**: The official Apache image in KRaft mode removes an entire container (ZooKeeper) and
its failure modes from the local stack, which directly serves SC-001 (a newcomer reaching a ready
state within 10 minutes). A single broker is correct for a local demo: the project's reliability
claims concern consumer group behavior and idempotent processing, not broker replication, and the
README's non-goals exclude consensus work.

**Alternatives considered**:

- `bitnami/kafka` — convenient environment variable conventions, but adds a vendor layer over the
  upstream configuration that contributors then have to learn.
- `confluentinc/cp-kafka` — the most widely documented, but historically ZooKeeper-coupled in
  examples and heavier to start.

---

## R-006: Readiness probe check semantics

**Decision**: Protocol-level liveness checks against each dependency, not raw TCP dials:
Kafka via a `kadm` metadata request, ClickHouse via `conn.Ping`, Redis via `PING`. Each runs under
its own `context.WithTimeout` (default 2s), all three run concurrently, and the aggregate result is
cached for a minimum re-check interval (default 1s) guarded by `singleflight` so concurrent probe
requests collapse into one round of dependency checks.

**Rationale**: The spec's assumption is a connection that "can be established and answered", which a
TCP dial does not satisfy — a listening socket on a broker that cannot serve metadata still accepts
connections. Running the three checks concurrently keeps worst-case probe latency at one timeout
rather than three, which matters for SC-002's 10-second window. The cache plus `singleflight`
combination is what satisfies FR-010: without it, a Kubernetes readiness probe at 1s intervals
across N replicas multiplies into N queries per second against every dependency, and this feature's
own load generator in F09 would be measuring probe traffic as well as real traffic.

**Alternatives considered**:

- A background goroutine refreshing health on a ticker, with the handler reading the last known
  result — constant background load even when nothing is probing, and the first probe after startup
  returns a stale-or-empty result.
- Checking dependencies sequentially and short-circuiting on first failure — cheaper in the failure
  case but violates FR-008, which requires every failing dependency to be listed, and triples
  worst-case latency in the timeout case.

---

## R-007: Configuration loading

**Decision**: A hand-written `internal/config` package (~200 lines) with a small typed-getter
helper per kind (string, int, duration, bool, string-slice), collecting **all** validation errors
before returning rather than failing on the first, and a `String()` method that masks fields tagged
sensitive. No third-party configuration library.

**Rationale**: FR-016 requires each error to name the setting, explain why the received value is
invalid, and state the permitted format or range, and SC-005 requires a reader to determine what to
change without opening the source. Struct-tag-driven libraries produce messages shaped by the
library, not by this requirement, and typically abort on the first failure — which turns a
three-mistake `.env` into three sequential restart-and-retry cycles. Accumulating errors and owning
the message text is the whole requirement, and it is roughly a day of work with no dependency.

**Alternatives considered**:

- `caarlos0/env` — clean struct tags and a small dependency, but error text is not controllable to
  the standard FR-016 sets.
- `spf13/viper` — brings file formats, remote config, and live reload, none of which this project
  wants; a clear Principle V violation.

---

## R-008: Structured logging and correlation identifier **(binding for later features)**

**Decision**: `log/slog` with `slog.NewJSONHandler` writing to stdout, wrapped in a custom
`slog.Handler` that reads a correlation ID from `context.Context` and injects it as `trace_id` on
every record. Service name and version are attached once via `logger.With`. The correlation ID is a
16-byte value rendered as 32 lowercase hex characters — identical to a W3C Trace Context trace ID.
Inbound HTTP requests adopt an incoming `traceparent` header's trace ID when present, otherwise a
new one is generated.

**Rationale**: Using the W3C trace ID shape now is what makes F08 a drop-in: when OpenTelemetry is
introduced, `trace.SpanContextFromContext(ctx).TraceID()` produces exactly this value, so the
handler's extraction function changes but the log field's name, format, and meaning do not. Picking
any other format (UUIDv4, a counter) would force every log query, dashboard, and saved search built
between now and F08 to be rewritten. Writing to stdout leaves collection to the container runtime,
which the spec explicitly scopes out.

**Alternatives considered**:

- `zerolog` / `zap` — faster at extreme volume, but `slog` is stdlib, has a context-aware handler
  interface designed for exactly this, and the API services here are not log-throughput bound.
- UUIDv4 correlation IDs — familiar, but incompatible with the trace ID format, guaranteeing a
  migration at F08.

---

## R-009: Graceful shutdown mechanics

**Decision**: `signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)` establishes the root
context. On cancellation the service (a) flips a readiness gate to "shutting down" so
`/v1/health/ready` immediately reports not-ready, (b) calls `http.Server.Shutdown` with a context
bounded by the configurable grace period (default 30s), (c) closes dependency clients, then exits 0.
A second signal — detected by re-arming `signal.Notify` on a buffered channel after the first —
exits immediately with a non-zero status. Grace period expiry logs a `shutdown_timeout` event and
forces exit.

**Rationale**: `http.Server.Shutdown` is precisely the "stop accepting new connections, let in-flight
requests finish" primitive FR-019 describes. Flipping readiness *before* shutdown begins (FR-020) is
what lets an orchestrator drain traffic; doing it after would race with the load balancer. The
second-signal path (FR-023) matters in practice because a developer who sends Ctrl-C twice expects
the process to die, and because Kubernetes sends SIGKILL after the grace period anyway — a process
that ignores repeated signals just looks hung.

**Alternatives considered**:

- A shutdown "lame duck" delay (report not-ready, then wait a fixed period before closing) — the
  correct pattern behind a load balancer with slow endpoint propagation, but it adds a fixed delay
  to every local restart and this feature has no load balancer yet. Reconsider in F11.

---

## R-010: Metrics scope for this feature

**Decision**: Use `prometheus/client_golang` with a non-global `prometheus.Registry` created in
`internal/observability` and passed explicitly. This feature registers: Go runtime and process
collectors, a `pulseflow_build_info` gauge (version, commit, Go version), and — because
constitution Principle I requires every processing path to be instrumented in the change that
introduces it — `pulseflow_dependency_up{dependency}` and
`pulseflow_dependency_check_duration_seconds{dependency}` for the readiness checks, plus
`pulseflow_http_requests_total{route,method,code}` and
`pulseflow_http_request_duration_seconds{route,method}` covering the health and metrics routes.

**Rationale**: FR-032 sets a floor ("at minimum"), not a ceiling, so instrumenting the readiness
check path here is consistent with the spec while satisfying Principle I. The `route` label uses the
registered pattern (`/v1/health/ready`) rather than the raw request path, keeping cardinality
bounded as Principle I requires. Using an explicit registry rather than
`prometheus.DefaultRegisterer` means tests can construct an isolated registry and assert on metric
names without global state leaking between test cases — which is what FR/SC coverage of "metric
names remain stable" needs in F08.

**Alternatives considered**:

- Deferring all metrics to F08 — cleaner feature boundary, but leaves this feature's readiness path
  uninstrumented, a direct Principle I violation.
- OpenTelemetry metrics SDK with a Prometheus exporter — where F08 may end up for traces, but adds a
  second metrics abstraction now for endpoints that `client_golang` already covers directly.

---

## R-011: Lint and tool version pinning

**Decision**: Pin `golangci-lint` as a module tool dependency using the `go.mod` `tool` directive
(`go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint`), invoked as
`go tool golangci-lint run`. CI runs the same command rather than a separate action.

**Rationale**: `golangci-lint` is not installed on the development machine observed here, and
FR-036 requires the pipeline's checks to be reproducible locally with one command. The `tool`
directive records the exact version in `go.mod`/`go.sum`, so local and CI runs cannot drift — which
is the same reproducibility argument constitution Principle III makes about benchmarks, applied to
tooling. Using the same invocation in CI removes the class of failure where CI lints with different
rules than the developer just ran.

**Alternatives considered**:

- `golangci/golangci-lint-action` in CI plus a manual local install — the common setup, but the
  action's version and the contributor's local binary drift, producing "passes locally, fails in CI".
- `go vet` only — no dependency at all, but misses the unused-result, shadowed-error, and
  unclosed-resource classes that matter most in the Kafka and ClickHouse code arriving in F02–F04.

---

## R-012: Test strategy and integration test gating

**Decision**: Standard library `testing` only, no assertion framework. Unit tests run by default.
Integration tests live in `tests/integration/`, carry the `//go:build integration` tag, read
dependency addresses from the same environment variables the services use, and run via
`make test-integration` against the compose stack. CI runs unit tests on every push and integration
tests in a job that first brings up compose.

**Rationale**: Constitution Principle II permits either testcontainers or compose for real-service
integration tests. Compose is the right choice for this feature because the stack already has to
exist and be verified (FR-001), so the integration test is testing the artifact the developer
actually uses. The build tag keeps `go test ./...` fast and dependency-free for the unit layer,
which is what most edit-compile-test cycles need. testcontainers is worth revisiting at F03/F04
where tests want an isolated, disposable ClickHouse per test rather than a shared one.

**Alternatives considered**:

- `testify` — better failure messages and `require` semantics, and it is nearly universal in Go
  projects. Rejected for this feature only on Principle V grounds; if assertion verbosity becomes a
  real friction point in F03's fixture comparisons, adding it there is a small, justified change.
- testcontainers-go from the start — stronger isolation, but starting a Kafka container per test
  package makes the F01 suite slower than the whole rest of the feature's build.

---

## R-013: Compose healthcheck definitions

**Decision**: Every supporting service declares a healthcheck, and the two application services
declare `depends_on: { condition: service_healthy }`:

| Service | Healthcheck |
| --- | --- |
| Kafka | `/opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list` |
| ClickHouse | `wget -qO- http://localhost:8123/ping` |
| Redis | `redis-cli ping` |
| Prometheus | `wget -qO- http://localhost:9090/-/healthy` |

**Rationale**: FR-002 requires distinguishing "container created" from "service available", which is
exactly what these commands test — each exercises the service's own protocol rather than its socket.
This is what makes SC-001's single-command startup deterministic instead of racy.

**Note on ordering**: `depends_on: service_healthy` improves the developer experience but is
deliberately **not** the mechanism the application services rely on for correctness. Per the spec's
assumption and the container-startup-race edge case, both services must start successfully and
report not-ready when dependencies are absent (FR-011, and the readiness contract generally). The
integration test asserts this by starting a service with the stack down.

---

## R-014: Image version pinning

**Decision**: Every image in `docker-compose.yml` is pinned to an explicit patch-level tag
(`apache/kafka:4.1.0`, `clickhouse/clickhouse-server:25.8`, `redis:8.2-alpine`,
`prom/prometheus:v3.6.0`), never `latest` or a floating major tag. Versions are recorded in a table
in the quickstart so upgrades are a deliberate, reviewable change.

**Rationale**: FR-003 requires it, and constitution Principle III depends on it: a benchmark result
measured against `clickhouse:latest` is not reproducible six months later, which would silently
invalidate the numbers this project exists to produce.

---

## Open items carried into implementation

None. No `NEEDS CLARIFICATION` markers remain in the Technical Context.
