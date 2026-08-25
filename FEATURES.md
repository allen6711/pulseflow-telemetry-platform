# PulseFlow Feature Breakdown (Spec Kit Development Guide)

This document splits the MVP scope defined in `README.md` into **11 features that can each be
specified, planned, and implemented independently**, along with copy-paste-ready
`/speckit.specify` prompts, dependency relationships, and acceptance criteria.

- Basis for the split: README sections "Core MVP" §1–§6, "MVP acceptance criteria", "Testing",
  "Benchmark plan", and "Stretch goals"
- Target workflow: `/speckit.constitution` → for each feature, one pass of `/speckit.specify` →
  `/speckit.clarify` → `/speckit.plan` → `/speckit.tasks` → `/speckit.analyze` → `/speckit.implement`

---

## 0. Before You Start: Write the Constitution

`.specify/memory/constitution.md` is the gate that every `plan.md` is checked against, so it must
be filled in before the first feature is opened. Run `/speckit.constitution` and cover at least
the following principles:

1. **Observability First** — every new processing path must emit metrics and structured logs
   (including a trace ID) in the same change that introduces it.
2. **Layered Testing (non-negotiable)** — pure logic gets unit tests; anything crossing a
   Kafka / ClickHouse / Redis boundary gets integration tests against real services
   (testcontainers or compose).
3. **Reproducible Measurement** — every performance number must come from a committed script and
   configuration, and must record hardware, command, dataset, duration, and raw output. Target
   values from the README must not be written as achieved results before measurement.
4. **At-Least-Once Delivery + Application-Level Idempotency** — persist before committing the
   offset; a duplicate `event_id` must never produce a second analytical record.
5. **Simplicity First** — do not implement Kafka / ClickHouse / Redis from scratch, no consensus
   algorithms, no microservice sprawl (see README "Explicit non-goals").

> Status: ✅ Ratified as constitution v1.0.0 on 2026-08-24.

---

## 1. Splitting Principles

| Principle | Description |
| --- | --- |
| Vertical slices | Apart from F01 (foundation), every feature is independently observable and verifiable once complete — you can call an API, see data land in ClickHouse, or watch a metric move. |
| One spec per feature | Each feature fits roughly into one spec with 3–8 user stories, preventing a single feature from swallowing the whole pipeline. |
| One-directional dependencies | Later features depend only on earlier ones and never break their contracts. When modifying an existing component is unavoidable (e.g. centralized instrumentation), the feature's spec must explicitly list the affected files and scope. |
| Cross-cutting concerns defined once | Configuration loading, logging, and the metric registry are built as shared packages in F01; later features only register what they need rather than rebuilding the plumbing. |
| Benchmarks and acceptance are their own features | The README treats measurement as a deliverable, so it needs its own spec and tasks rather than being scattered as TODOs. |

---

## 2. Dependency Graph

```text
F01 Platform Foundation & Local Environment
 ├── F02 Event Ingestion API ──────────────┐
 ├── F03 ClickHouse Analytical Store ──────┼── F04 Partitioned Worker
 │                                         │        │
 │                                         │        └── F05 Retry / DLQ / Idempotency
 │                                         │
 │                                         └── F06 Analytics Query API
 │                                                  │
 │                                                  └── F07 Redis Query Cache
 │
 └── F08 Observability (OTel + full Prometheus metric catalog)
              │
              ├── F09 Load Generator & Benchmark Suite
              │        │
              │        └── F10 E2E Tests & MVP Acceptance
              │
              └── F11 Kubernetes Deployment (post-MVP)

F12+ Stretch goals (only after MVP acceptance passes)
```

**Critical path to the shortest demonstrable demo**: F01 → F02 → F03 → F04 → F06.
Those five give you the complete chain of "send an event → persist it → query the aggregate".
Demo that first, then layer on reliability and performance.

---

## 3. Feature List

### F01 · Platform Foundation & Local Development Environment

- **Suggested branch / short-name**: `platform-foundation`
- **README mapping**: Repository layout, MVP acceptance criterion #1, API summary
  `/v1/health/live` and `/v1/health/ready`
- **Depends on**: nothing

**In scope**

- Go module plus `cmd/api` and `cmd/worker` skeletons (start cleanly, shut down gracefully)
- `internal/config`: environment-variable-driven configuration with defaults and startup validation
  (Kafka brokers, ClickHouse DSN, Redis address, port, log level)
- Structured JSON logging package (`log/slog`) with service, version, level, and a `trace_id` field
- `docker-compose.yml`: Kafka, ClickHouse, Redis, Prometheus, each with a healthcheck and a pinned
  image version
- `/v1/health/live` (pure liveness) and `/v1/health/ready` (dependency-aware readiness across
  Kafka / ClickHouse / Redis)
- `Makefile` or `scripts/`: build, test, lint, compose up/down
- GitHub Actions CI: build + vet + unit tests

**Out of scope**: any business logic, actual metric content (registry skeleton only), Kubernetes.

**Acceptance**

- After `docker compose up`, all dependencies report healthy and the API and worker containers
  start without crashing
- With ClickHouse stopped, `/v1/health/ready` returns 503 and names the failing dependency, while
  `/v1/health/live` still returns 200
- CI is green on pull requests

```text
/speckit.specify --short-name platform-foundation
Build the PulseFlow project skeleton and local development environment. This includes: a Go module with two runnable binaries, cmd/api and cmd/worker, both supporting graceful shutdown; internal/config providing environment-variable-driven configuration with sensible defaults and startup-time validation; a structured JSON logging package built on log/slog that reserves a trace_id field; a docker-compose.yml that starts Kafka, ClickHouse, Redis, and Prometheus with healthchecks and pinned image versions; an API exposing /v1/health/live as a pure liveness probe and /v1/health/ready as a dependency-aware readiness probe that checks Kafka, ClickHouse, and Redis and returns 503 listing the failing dependencies; a Makefile providing build, test, lint, and compose lifecycle commands; and a GitHub Actions workflow running build, go vet, and unit tests. This feature includes no business logic and no Kubernetes deployment.
```

> Status: ✅ Spec written at `specs/001-platform-foundation/spec.md` (quality checklist passed
> on the first pass, 0 open clarifications).

---

### F02 · Event Ingestion API

- **Suggested branch / short-name**: `event-ingestion-api`
- **README mapping**: Core MVP §1, API summary `POST /v1/events` and `POST /v1/events/batch`
- **Depends on**: F01

**In scope**

- The canonical event schema (`event_id`, `service`, `timestamp`, `metric`, `value`, `tags`,
  schema version) and its validation rules
- `POST /v1/events`: validate → publish to the Kafka topic `telemetry.events` → return
  `202 Accepted` only after Kafka acknowledges
- `POST /v1/events/batch`: a bounded batch (e.g. 1000 events) with clearly defined partial-failure
  semantics
- Kafka producer: partition key derived from `service` (or `service+metric`) so events from one
  service stay ordered
- Error mapping: 400 (invalid schema), 413 (batch too large), 503 (Kafka unavailable), and a
  uniform JSON error shape
- Unit tests for schema validation and error mapping; integration test for the producer against
  real Kafka

**Out of scope**: the consumer side, ClickHouse, caching, rate limiting (stretch).

**Acceptance**

- A valid event returns 202 and can be read back from `telemetry.events` with a console consumer
- Missing fields, wrong types, and malformed timestamps each return 400 and name the offending field
- With Kafka down the API returns 503 and never returns 202 (no false success)
- A batch above the limit returns 413

```text
/speckit.specify --short-name event-ingestion-api
Implement the PulseFlow telemetry event ingestion API. Define the canonical event schema (event_id, service, timestamp, metric, value, tags, schema version) and its validation rules. POST /v1/events accepts a single event, validates it, publishes it to the Kafka topic telemetry.events, and returns 202 Accepted only after Kafka has acknowledged the write. POST /v1/events/batch accepts a bounded batch with a default limit of 1000 events and must define partial-success and partial-failure response semantics explicitly. The Kafka producer uses service as the partition key so that events from a single service preserve ordering. Error mapping must cover: invalid schema returns 400 and names the offending field, an oversized batch returns 413, and Kafka being unavailable returns 503 and never 202. Include unit tests for schema validation and error mapping, plus a producer integration test against real Kafka. Exclude the consumer side, ClickHouse persistence, and rate limiting.
```

---

### F03 · ClickHouse Analytical Store

- **Suggested branch / short-name**: `clickhouse-store`
- **README mapping**: Core MVP §2 (persist), §3 (aggregates), Testing "ClickHouse repository
  integration test"
- **Depends on**: F01

**In scope**

- `migrations/`: table design and a repeatable migration mechanism
  - Detail table partitioned by time, with a sort key over `service`, `metric`, and timestamp that
    supports time-range scans
  - A data-layer foundation for idempotency (e.g. `ReplacingMergeTree` keyed on `event_id`, or a
    dedicated dedupe table)
  - Materialized views or pre-aggregation tables where needed to support percentile queries
- `internal/storage`: repository interface plus ClickHouse implementation (batch writes,
  aggregation queries, service listing)
- Batch write strategy: batch size, flush interval, timeout
- Integration test against real ClickHouse verifying writes and aggregation correctness using a
  known fixture

**Out of scope**: the Kafka consumption loop, the HTTP layer.

**Acceptance**

- Migrations run successfully against a clean ClickHouse and are safe to re-run
- After writing a fixed fixture, count / avg / min / max / p50 / p95 / p99 queries return the
  expected values
- Writing the same `event_id` twice still yields a single logical record under the dedupe strategy

```text
/speckit.specify --short-name clickhouse-store
Build the PulseFlow ClickHouse analytical storage layer. Include a migrations directory and a repeatable migration mechanism. Design a telemetry detail table partitioned by time with a sort key over service, metric, and timestamp to support time-range scans. Use a table engine strategy that supports deduplication by event_id as the data-layer foundation for idempotency. Add pre-aggregation tables or materialized views where query patterns require them to support percentile queries. Implement a repository interface in internal/storage with a ClickHouse implementation providing batch writes, time-range aggregation queries (count, avg, min, max, p50, p95, p99), and a query for the list of observed services, with a defined batch size, flush interval, and timeout strategy. Include an integration test against real ClickHouse that uses a known fixture to verify aggregate correctness and verifies that a duplicate event_id is not counted twice. Exclude the Kafka consumption loop and the HTTP layer.
```

---

### F04 · Partitioned Worker (Consumer Group + Persistence)

- **Suggested branch / short-name**: `partitioned-worker`
- **README mapping**: Core MVP §2 (first half), MVP acceptance "at least three worker replicas
  share processing"
- **Depends on**: F02, F03

**In scope**

- Kafka consumer group consuming `telemetry.events`, with multiple replicas processing different
  partitions in parallel
- Event schema and version validation; malformed events are rejected (recorded and counted at this
  stage)
- Deduplication by `event_id` (application level plus the data-layer strategy from F03)
- Batch writes to ClickHouse
- **Commit offsets only after successful persistence** (at-least-once)
- Consumer rebalance handling and graceful shutdown (flush before exiting on SIGTERM)
- Integration tests: Kafka producer/consumer, and duplicate `event_id` producing exactly one
  analytical record

**Out of scope**: retry backoff and DLQ (F05), the full metric catalog (F08).

**Acceptance**

- With three worker replicas running, partitions are distributed evenly and total processed volume
  equals what was submitted (minus duplicates)
- Feeding a dataset containing duplicate `event_id`s yields exactly one valid analytical record
  per ID in ClickHouse
- Killing a worker before it flushes causes the batch to be reprocessed after restart rather than
  lost

```text
/speckit.specify --short-name partitioned-worker
Implement the PulseFlow telemetry processing worker. The worker consumes telemetry.events using a Kafka consumer group, supports multiple replicas processing different partitions in parallel, and scales horizontally. The processing pipeline validates event schema and version, rejects malformed events, deduplicates by event_id, writes to ClickHouse in batches, and commits Kafka offsets only after persistence succeeds, giving at-least-once delivery semantics. It must handle consumer group rebalances and, on SIGTERM, flush any pending batch before exiting. Include Kafka producer/consumer integration tests, a test verifying that a duplicate event_id produces exactly one analytical record, and verification that three replicas can share consumption of the same topic. This feature excludes retry backoff and dead-letter handling, and excludes the full metric catalog.
```

---

### F05 · Reliability: Retry Classification, Backoff, and Dead-Letter

- **Suggested branch / short-name**: `retry-and-dlq`
- **README mapping**: Core MVP §2 (second half), §5
- **Depends on**: F04

**In scope**

- Failure classification: transient (ClickHouse timeout, network) vs permanent (invalid schema,
  unparseable)
- Bounded exponential backoff for transient failures (configurable attempts, base, cap, jitter)
- Messages exceeding the retry limit or classified as permanent go to a dead-letter topic
- DLQ record format: original payload + failure reason + error class + attempt count + first and
  last failure time + trace ID + source partition/offset
- A way to inspect the DLQ (script or small CLI)
- Tests: unit tests for retry classification, integration test injecting failures to drive messages
  into the DLQ
- Worker restart / rebalance drill script (reused later by the F09 failure experiment)

**Out of scope**: automatic DLQ replay (candidate stretch goal).

**Acceptance**

- With an injected transient error, the message succeeds after backoff and never reaches the DLQ
- With an injected permanent error (poison message), the message reaches the DLQ within the retry
  limit, and the DLQ record exposes the original payload and failure metadata
- Across a drill of 20 worker terminations mid-processing, no Kafka-acknowledged event is lost

```text
/speckit.specify --short-name retry-and-dlq
Add reliability handling to the PulseFlow worker. Classify processing failures as transient (ClickHouse timeouts, network errors) or permanent (invalid schema, unparseable payload). Retry transient failures with bounded exponential backoff where attempt count, base interval, cap, and jitter are all configurable. Route messages that exceed the retry limit or are classified as permanent to a dead-letter topic. DLQ records must contain the original payload, failure reason, error classification, attempt count, first and last failure timestamps, trace ID, and source partition and offset, and a script must exist to inspect DLQ contents. Include unit tests for retry classification, an integration test that injects failures to verify messages land in the DLQ, and a repeatable worker termination and restart drill script used to verify that Kafka-acknowledged events are not lost across restarts. Exclude automatic DLQ replay.
```

---

### F06 · Analytics Query API

- **Suggested branch / short-name**: `analytics-query-api`
- **README mapping**: Core MVP §3, API summary `GET /v1/metrics/{service}/{metric}` and
  `GET /v1/services`
- **Depends on**: F03 (can run in parallel with F04)

**In scope**

- `GET /v1/metrics/{service}/{metric}?from=&to=&percentiles=p50,p95,p99`
- Response includes count, avg, min, max, and the requested percentiles
- Query parameter validation: timestamp format, `from < to`, maximum time range, allowed
  percentile list, handling of unknown parameters
- `GET /v1/services`: list observed services (optionally with their metrics)
- Error mapping: 400 (invalid parameters), well-defined 404-or-empty-result semantics,
  504 (query timeout)
- Unit tests for query validation and API error mapping; integration test asserting correct
  aggregates against a known fixture

**Out of scope**: caching (F07).

**Acceptance**

- Queries against a known fixture return correct aggregates (matching the expectations in the F03
  integration test)
- Invalid time ranges and percentiles return 400 with an explanation
- Queries exceeding the maximum time range are rejected rather than allowed to overload ClickHouse

```text
/speckit.specify --short-name analytics-query-api
Implement the PulseFlow analytics query API. GET /v1/metrics/{service}/{metric} supports from and to time-range parameters and a percentiles parameter (such as p50,p95,p99), and returns count, average, minimum, maximum, and the requested percentiles. Validate query parameters: timestamp format, from must precede to, enforce a maximum queryable time range, restrict percentiles to an allowed list, and define how unknown parameters are handled. GET /v1/services lists observed services and their metrics. Error mapping must cover invalid parameters returning 400 with an explanation, explicitly defined response semantics when no data exists, and query timeouts returning 504. Include unit tests for query parameter validation and API error mapping, plus an integration test asserting correct aggregate values against a known fixture. This feature excludes Redis caching.
```

---

### F07 · Redis Query Cache

- **Suggested branch / short-name**: `redis-query-cache`
- **README mapping**: Core MVP §4, Testing "cache-key generation" and "Redis integration test"
- **Depends on**: F06

**In scope**

- Deterministic cache key derived from service, metric, normalized time range, and aggregation
  options
- Time-range normalization strategy (e.g. aligning to bucket boundaries to raise hit rate) and how
  queries whose range includes "now" are handled
- TTL with randomized jitter so keys do not all expire simultaneously
- Cache hit / miss metrics and hit ratio
- A cache bypass mechanism (e.g. `?no_cache=1` or a header) for benchmarking
- Unit tests for cache key determinism and collision safety; integration test against real Redis
- Degrade to querying ClickHouse directly when Redis is unavailable (the query must not fail
  outright)

**Out of scope**: distributed locking / singleflight (stretch, though it may be included if it can
be kept simple).

**Acceptance**

- A repeated identical query hits the cache and returns content identical to the uncached response
- Semantically equivalent queries with differently ordered parameters produce the same key;
  semantically different queries never share a key
- TTL jitter is observable as spread-out expiry times
- Queries still succeed with Redis down, and the degradation is logged

```text
/speckit.specify --short-name redis-query-cache
Add a Redis cache layer to PulseFlow analytics queries. Derive a deterministic cache key from service, metric, normalized time range, and aggregation options, such that semantically equivalent queries with differently ordered parameters produce the same key while semantically different queries never share a key; define the time-range alignment strategy and how queries whose range includes the current time are handled. Cached entries use a TTL with randomized jitter so that many keys do not expire at the same instant. Expose cache hit, cache miss, and hit-ratio metrics, and provide a mechanism to bypass the cache for benchmarking. When Redis is unavailable the system must degrade to querying ClickHouse directly and log the degradation rather than failing the query. Include unit tests for cache key determinism and an integration test against real Redis.
```

---

### F08 · Observability: OpenTelemetry Tracing and the Prometheus Metric Catalog

- **Suggested branch / short-name**: `observability`
- **README mapping**: Core MVP §6, API summary `/metrics`
- **Depends on**: F02, F04, F06 (the components must exist before they can be fully instrumented)

**In scope**

- The complete metric catalog (all ten from README §6): ingestion RPS, ingestion p50/p95/p99,
  Kafka publish failures, consumer lag, processed events/sec, processing failures, dead-letter
  count, ClickHouse write latency, analytics query p95, Redis hit ratio
- `/metrics` Prometheus endpoint exposed by both the API and the worker
- Metric naming conventions and label cardinality control (never use `event_id` or high-cardinality
  tags as labels)
- OpenTelemetry traces: context propagation from HTTP ingestion through the Kafka publish (via
  Kafka headers), with the worker continuing the span
- Trace IDs carried into structured logs
- Prometheus scrape configuration added to compose
- Tests asserting metric existence and name stability

**Cross-cutting rule**: F02–F07 only register the metrics they strictly need; F08 owns unifying the
naming, filling the gaps, and adding tracing, and its spec must explicitly list which existing files
it modifies.

**Acceptance**

- `curl :PORT/metrics` shows all ten metrics with values that actually move
- A single ingestion request produces a trace where the HTTP span and the Kafka publish span are
  connected, and the worker span carries the same trace ID
- Trace IDs in logs can be matched back to their trace

```text
/speckit.specify --short-name observability
Build complete observability for PulseFlow. Expose the following Prometheus metrics: ingestion request rate, ingestion p50/p95/p99 latency, Kafka publish failures, consumer lag, processed events per second, processing failures, dead-letter count, ClickHouse write latency, analytics query p95 latency, and Redis cache hit ratio. Both the API and the worker expose their own /metrics endpoint. Define metric naming conventions and label cardinality rules that forbid using event_id or high-cardinality tags as labels. Use OpenTelemetry for tracing, propagating trace context from HTTP ingestion through Kafka headers so that worker spans belong to the same trace, and emit trace IDs in structured logs. Prometheus scrape configuration must be added to docker compose. Include tests verifying that metrics exist and their names remain stable. This feature modifies the existing API, worker, storage, and cache components to add instrumentation.
```

---

### F09 · Load Generator and Benchmark Suite

- **Suggested branch / short-name**: `benchmark-suite`
- **README mapping**: Benchmark plan (four experiments), MVP acceptance "one million events" and
  "benchmark scripts and results committed"
- **Depends on**: F07, F08

**In scope**

- Synthetic event generator: configurable service count, metric count, total events, rate,
  duplicate `event_id` ratio, poison message ratio
- k6 scripts for ingestion load and analytics query load
- Repeatable scripts for the four experiments:
  1. Throughput scaling (1 / 2 / 4 workers)
  2. Query caching (uncached vs cached p95 comparison)
  3. Worker failure (terminate a worker mid-processing and verify recovery)
  4. Duplicate delivery (resend event IDs and verify exactly one analytical record per ID)
- Result collection: pull metrics from Prometheus, emit raw data and a summary
- `benchmarks/README.md`: the recording format for hardware, command, dataset, configuration, run
  duration, and raw/summary results (values left blank until measured)

**Out of scope**: writing target values as if they were results (explicitly forbidden by the README).

**Acceptance**

- One million events can be generated and submitted by a single command with no manual intervention
- Each of the four experiments has a repeatable script producing committed result files
- The caching experiment yields a measurable p95 difference

```text
/speckit.specify --short-name benchmark-suite
Build the PulseFlow load generator and benchmark suite. Provide a synthetic telemetry event generator with configurable service count, metric count, total event count, send rate, duplicate event_id ratio, and poison message ratio, capable of generating and submitting at least one million events via a single command with no manual intervention. Write k6 scripts for ingestion load and analytics query load. Provide four repeatable experiment scripts: a throughput scaling experiment running the same workload with 1, 2, and 4 workers; a caching experiment comparing uncached and cached analytics query p95 latency; a failure experiment that terminates a worker during ingestion and verifies recovery; and a duplicate delivery experiment that intentionally resends event IDs and verifies exactly one analytical record per ID. Results must be collected from Prometheus and written out as raw data and summary files. The benchmarks directory needs documentation recording hardware, commands, dataset, configuration, run duration, and result format; until real measurements exist, target values must not be written as achieved results.
```

---

### F10 · End-to-End Tests and MVP Acceptance

- **Suggested branch / short-name**: `e2e-acceptance`
- **README mapping**: Testing "End-to-end tests must exercise the local container stack", all ten
  MVP acceptance criteria
- **Depends on**: F09

**In scope**

- E2E tests exercising ingest → process → query against the full compose stack
- Coverage of the DLQ path, the duplicate-event path, and the cache path
- An "MVP acceptance" script that checks each of the README's ten acceptance criteria and emits a
  pass/fail report
- E2E running in CI (with a reduced dataset)

**Acceptance**

- The acceptance script runs from scratch on a clean environment and all ten criteria pass, with a
  report produced

```text
/speckit.specify --short-name e2e-acceptance
Build end-to-end tests and the MVP acceptance process for PulseFlow. E2E tests run against the full stack started by docker compose and exercise the complete chain from event ingestion through worker processing to analytics queries, covering the dead-letter path, the duplicate-event path, and the cache-hit path. Provide an MVP acceptance script that checks each of the ten acceptance criteria listed in the README: compose starts all dependencies, one million events can be submitted without manual intervention, at least three worker replicas share consumption, duplicate event IDs do not create duplicate analytical records, failed messages reach the dead-letter topic after bounded retries, analytics queries return correct aggregates for a known fixture, Redis produces a measurable query latency difference, Prometheus exposes throughput, lag, failure, and latency metrics, worker termination tests lose no acknowledged events, and benchmark scripts and results are committed — emitting a per-criterion pass or fail report. E2E tests must be runnable in CI with a reduced dataset.
```

---

### F11 · Kubernetes Deployment (Post-MVP)

- **Suggested branch / short-name**: `k8s-deployment`
- **README mapping**: Planned technology stack (Kubernetes), Repository layout `deployments/k8s`
- **Depends on**: F10

**In scope**

- Deployment, Service, ConfigMap, and Secret definitions for the API and worker
- Liveness / readiness probes wired to the endpoints from F01
- Resource requests/limits and `terminationGracePeriodSeconds` (matching the worker's flush behavior)
- Adjustable worker replica count; PodDisruptionBudget
- ServiceMonitor or Prometheus scrape annotations
- Documented deployment and verification steps on kind / minikube

**Acceptance**

- Deploys successfully on a local kind cluster with readiness probes passing, and scaling the
  worker replica count triggers a consumer group rebalance without losing events

```text
/speckit.specify --short-name k8s-deployment
Create Kubernetes deployment configuration for PulseFlow. Provide Deployment, Service, ConfigMap, and Secret definitions for the API and the worker. Wire liveness and readiness probes to the existing /v1/health/live and /v1/health/ready endpoints. Set resource requests and limits, and choose terminationGracePeriodSeconds to match the worker's flush behavior. Make the worker replica count adjustable and define a PodDisruptionBudget. Provide configuration that lets Prometheus scrape metrics. Include documentation for deploying and verifying on a local kind or minikube cluster, and verify that changing the worker replica count triggers a consumer group rebalance without losing events.
```

---

## 4. Stretch Goals (Only After F10 Acceptance Passes)

The README explicitly requires these to come after MVP acceptance. Each should be its own feature:

| ID | Feature | short-name | Depends on |
| --- | --- | --- | --- |
| F12 | gRPC ingestion endpoint | `grpc-ingestion` | F02 |
| F13 | Kubernetes HPA driven by consumer lag | `lag-based-hpa` | F11 |
| F14 | Grafana dashboard | `grafana-dashboard` | F08 |
| F15 | Schema version compatibility tests | `schema-compat` | F02, F04 |
| F16 | Configurable retention / TTL policy | `retention-policy` | F03 |
| F17 | Per-tenant rate limits | `tenant-rate-limits` | F02 |

---

## 5. Suggested Milestones

| Milestone | Contents | Demonstrable outcome |
| --- | --- | --- |
| M1 Skeleton | F01 | Compose comes up; health checks are dependency-aware |
| M2 Minimal end-to-end chain | F02 + F03 + F04 + F06 | Send an event → land in ClickHouse → query correct aggregates |
| M3 Production-grade reliability and performance | F05 + F07 + F08 | Retries, DLQ, caching, full metrics and tracing |
| M4 Measurable and acceptable | F09 + F10 | Four benchmarks with real numbers; all ten acceptance criteria pass |
| M5 Deployment | F11 | Runs on kind; worker scales |
| M6+ | F12–F17 | Optional |

---

## 6. Suggested Per-Feature Workflow

```bash
# 1) Once only
/speckit.constitution

# 2) Repeat per feature
/speckit.specify --short-name <short-name>  <the matching prompt above>
/speckit.clarify      # resolve open design trade-offs (batch limits, TTLs, retry counts, ...)
/speckit.plan         # produce the technical plan; checked against the constitution
/speckit.tasks        # generate the executable task list
/speckit.analyze      # check spec / plan / tasks consistency
/speckit.implement    # execute
/speckit.checklist    # when extra quality gates are warranted (e.g. F09, F10)
```

**Features where `/speckit.clarify` is especially worth running**: F02 (batch limit and
partial-failure semantics), F05 (retry counts and backoff parameters), F07 (time-range
normalization and TTL strategy), F09 (benchmark hardware and dataset definitions).
