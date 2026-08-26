---
description: "Task list for 001-platform-foundation"
---

# Tasks: Platform Foundation & Local Development Environment

**Input**: Design documents from `/specs/001-platform-foundation/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: Test tasks are **included and mandatory**. Constitution v1.0.1 Principle II (Layered
Testing) is marked NON-NEGOTIABLE, and the spec's Testing obligations are restated in the plan's
Constitution Check. Unit tests cover pure logic; integration tests carry `//go:build integration`
and run against the compose stack (R-012).

**Organization**: Tasks are grouped by user story so each story can be implemented, tested, and
checked off independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel — different files, no dependency on an incomplete task
- **[Story]**: Which user story the task belongs to (US1–US6)
- Every task names an exact file path

## Path Conventions

Multi-binary Go backend, paths relative to the repository root, per the Project Structure section of
[plan.md](./plan.md): binaries in `cmd/`, shared packages in `internal/`, integration tests in
`tests/integration/`, container assets in `deployments/docker/`.

---

## Note on phase ordering

Two deliberate deviations from the default template structure, both recorded here so
`/speckit.analyze` does not read them as inconsistencies:

1. **Foundational owns the *mechanisms*; the P2/P3 stories own the *contracts* on top of them.**
   `internal/config`, `internal/logging`, and `internal/observability` are dependencies of every
   other package, so their basic mechanism (load with defaults, emit JSON, hold a registry) is built
   in Phase 2. The behavior each story is actually about — multi-error validation with the FR-016
   message contract (US3), correlation ID propagation and level filtering (US5) — is built and
   tested in that story's own phase. Without this split, no story could compile; with it, each story
   remains an independently verifiable increment.

2. **US1's compose healthcheck for the two application containers starts on `/v1/health/live` and is
   upgraded to `/v1/health/ready` in US2 (T047).** US1's acceptance scenario says the services
   "report ready", which strictly requires the readiness endpoint from US2. Rather than pull US2's
   work forward, US1 ships with liveness-based container health — sufficient for `make up` to be
   deterministic — and US2 completes the picture. This is the only place where a P1 story's
   acceptance is partially satisfied at its own checkpoint.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Repository skeleton, dependencies, and tooling so that everything after this compiles
and lints.

- [ ] T001 Initialize the Go module with `go 1.25` and create the directory skeleton (`cmd/api`, `cmd/worker`, `internal/{config,logging,observability,health,httpserver,lifecycle}`, `tests/integration`, `deployments/docker`, `scripts`) in `go.mod`
- [ ] T002 Add runtime dependencies pinned per R-003/R-004/R-006/R-010 (`github.com/twmb/franz-go`, `github.com/twmb/franz-go/pkg/kadm`, `github.com/ClickHouse/clickhouse-go/v2`, `github.com/redis/go-redis/v9`, `github.com/prometheus/client_golang`, `golang.org/x/sync`) in `go.mod`
- [ ] T003 [P] Pin `golangci-lint` as a module tool dependency via the `go.mod` `tool` directive and add a baseline ruleset in `.golangci.yml` (R-011)
- [ ] T004 [P] Create the Makefile with `build`, `test`, `lint`, and `check` targets in `Makefile`
- [ ] T005 [P] Create a minimal CI workflow running build, `go vet`, and unit tests in `.github/workflows/ci.yml` (extended to full coverage in US6)
- [ ] T006 [P] Create a multi-stage build for the ingestion service in `deployments/docker/Dockerfile.api`
- [ ] T007 [P] Create a multi-stage build for the processing service in `deployments/docker/Dockerfile.worker`
- [ ] T008 [P] Document every environment variable with its default from `contracts/configuration.md` in `.env.example`

**Checkpoint**: `make build` and `make lint` succeed against empty `main` functions.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared mechanisms every user story depends on. See the phase-ordering note above
for what is deliberately deferred to US3 and US5.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T009 Define the `Config` struct with every field, type, default, and sensitive marker from `contracts/configuration.md` in `internal/config/config.go`
- [ ] T010 Implement typed environment getters for string, int, duration, bool, and comma-separated list in `internal/config/parse.go`
- [ ] T011 Implement `config.Load()` reading the environment and applying defaults in `internal/config/config.go` (depends on T009, T010)
- [ ] T012 [P] Implement `logging.New()` returning an `slog` JSON handler with `service` and `version` attributes in `internal/logging/logger.go`
- [ ] T013 [P] Implement `observability.NewRegistry()` with a non-global `prometheus.Registry`, the `pulseflow_build_info` gauge, and the Go runtime and process collectors in `internal/observability/registry.go`
- [ ] T014 Implement the HTTP server wrapper with `ServeMux` route registration and a `Shutdown` method in `internal/httpserver/server.go`
- [ ] T015 Implement the liveness handler returning the `LivenessResponse` shape from `contracts/health-api.yaml` in `internal/health/handler.go`
- [ ] T016 Register the `/metrics` route backed by `promhttp.HandlerFor` against the feature registry in `internal/httpserver/server.go` (depends on T013, T014)
- [ ] T017 [P] Implement the route-labelled request counter and duration histogram from `contracts/metrics.md` in `internal/observability/httpmetrics.go`
- [ ] T018 Implement the middleware chain (panic recovery, request logging, metrics) in `internal/httpserver/middleware.go` (depends on T017)
- [ ] T019 [P] Write a unit test asserting route registration and the liveness response shape in `internal/httpserver/server_test.go`
- [ ] T020 Wire config, logger, registry, server, and liveness into the ingestion binary in `cmd/api/main.go`
- [ ] T021 Wire the same lifecycle into the processing binary, registering only health and metrics routes, in `cmd/worker/main.go`

**Checkpoint**: Both binaries start, serve `/v1/health/live` and `/metrics`, and emit JSON logs.
User story work can now begin.

---

## Phase 3: User Story 1 - One Command Starts the Whole Local Environment (Priority: P1) 🎯 MVP

**Goal**: A developer on a clean machine runs one command and gets every supporting service plus
both application services, each reporting health through its own protocol.

**Independent Test**: On a machine with no supporting services installed, run `make up` and confirm
every service reaches healthy; run `make down` then `make up` again and confirm an identical result.

### Tests for User Story 1

> Write these first and confirm they fail before implementing.

- [ ] T022 [P] [US1] Write an integration test asserting every compose service reaches a healthy state in `tests/integration/compose_test.go`
- [ ] T023 [P] [US1] Write an integration test asserting repeated up/down cycles leave no residual state that changes the outcome in `tests/integration/idempotence_test.go`

### Implementation for User Story 1

- [ ] T024 [US1] Add the Kafka service using `apache/kafka:4.1.0` in KRaft mode with the `kafka-topics.sh --list` healthcheck from R-013 in `docker-compose.yml`
- [ ] T025 [US1] Add the ClickHouse service using `clickhouse/clickhouse-server:25.8` with the `/ping` healthcheck in `docker-compose.yml` (depends on T024)
- [ ] T026 [US1] Add the Redis (`redis:8.2-alpine`) and Prometheus (`prom/prometheus:v3.6.0`) services with their healthchecks in `docker-compose.yml` (depends on T025)
- [ ] T027 [US1] Add the api and worker services with `depends_on: service_healthy`, a `/v1/health/live` container healthcheck, and environment wiring in `docker-compose.yml` (depends on T026)
- [ ] T028 [P] [US1] Add the static scrape configuration targeting both application services in `deployments/docker/prometheus.yml`
- [ ] T029 [US1] Add the `up`, `down`, `ps`, and `logs` targets that wait for healthy in `Makefile`
- [ ] T030 [P] [US1] Write the cold-start verification script measuring time to all-healthy for SC-001 in `scripts/verify-cold-start.sh`
- [ ] T031 [P] [US1] Write the restart-idempotence script running five consecutive up/down cycles for SC-010 in `scripts/verify-restart-idempotence.sh`
- [ ] T032 [US1] Add the `test-integration` target running the `integration` build tag against the running stack in `Makefile` (depends on T029)
- [ ] T033 [US1] Record the pinned image version table and prerequisites in `README.md`

**Checkpoint**: `make up` brings the whole stack healthy from clean, deterministically and
repeatably. Container health for the application services is liveness-based until T047.

---

## Phase 4: User Story 2 - Health Probes Distinguish "Process Alive" from "Ready for Traffic" (Priority: P1)

**Goal**: Two probes with distinct meanings — liveness unaffected by dependencies, readiness
reflecting all three dependencies and naming every failure.

**Independent Test**: Stop each dependency in turn and confirm liveness stays 200 while readiness
returns 503 naming the failing dependency and still listing the healthy ones; restart the dependency
and confirm automatic recovery without a service restart.

### Tests for User Story 2

- [ ] T034 [P] [US2] Write unit tests for readiness aggregation across mixed dependency states, asserting all three entries are always present in `internal/health/aggregate_test.go`
- [ ] T035 [P] [US2] Write a unit test asserting the readiness response matches the `ReadinessResponse` schema and status code mapping in `contracts/health-api.yaml` in `internal/health/handler_test.go`
- [ ] T036 [P] [US2] Write a unit test asserting failure reasons use the bounded vocabulary and never contain raw driver error text in `internal/health/checker_test.go`
- [ ] T037 [P] [US2] Write an integration test exercising each dependency probe against the live stack in `tests/integration/deps_test.go`
- [ ] T038 [P] [US2] Write an integration test asserting readiness reports 503 with the correct failing dependency when each service is stopped in `tests/integration/deps_down_test.go`
- [ ] T039 [P] [US2] Write an integration test asserting a service started with the whole stack down still starts and reports 503 rather than exiting in `tests/integration/startup_test.go`

### Implementation for User Story 2

- [ ] T040 [US2] Define the `Checker` interface, the `DependencyHealthStatus` entity, and the bounded reason classification from `data-model.md` in `internal/health/checker.go`
- [ ] T041 [P] [US2] Implement the Kafka checker using a `kadm` metadata request in `internal/health/kafka.go`
- [ ] T042 [P] [US2] Implement the ClickHouse checker using the native-protocol `Ping` in `internal/health/clickhouse.go`
- [ ] T043 [P] [US2] Implement the Redis checker using `PING` in `internal/health/redis.go`
- [ ] T044 [US2] Implement concurrent fan-out across all checkers with a per-check timeout from `HealthCheckTimeout` in `internal/health/aggregate.go` (depends on T040–T043)
- [ ] T045 [US2] Add `singleflight` collapsing and the minimum re-check interval cache satisfying FR-010 in `internal/health/aggregate.go` (depends on T044)
- [ ] T046 [US2] Implement the readiness handler with 200/503 mapping and the full dependency list in `internal/health/handler.go` (depends on T045)
- [ ] T047 [US2] Implement `pulseflow_dependency_up` and `pulseflow_dependency_check_duration_seconds` per `contracts/metrics.md` in `internal/observability/depmetrics.go`
- [ ] T048 [US2] Wire readiness into both binaries and switch the application containers' compose healthcheck to `/v1/health/ready` in `cmd/api/main.go`, `cmd/worker/main.go`, and `docker-compose.yml`

**Checkpoint**: Both P1 stories complete. The stack starts from clean and reports accurate,
dependency-aware health. This is the demonstrable MVP.

---

## Phase 5: User Story 3 - Configuration Errors Fail at Startup (Priority: P2)

**Goal**: Every setting validated at startup, all failures reported at once with messages that name
the variable, the value received, and the permitted range.

**Independent Test**: Start the service with valid, missing, and multiple-invalid configurations and
confirm the outcome and message quality each time.

### Tests for User Story 3

- [ ] T049 [P] [US3] Write a unit test asserting three simultaneous configuration errors are all reported in one failure, not just the first in `internal/config/validate_test.go`
- [ ] T050 [P] [US3] Write a unit test asserting each error message names the variable, quotes the received value, and states the permitted range per `contracts/configuration.md` in `internal/config/errors_test.go`
- [ ] T051 [P] [US3] Write a unit test asserting the cross-field rule that `HealthCacheTTL` must be strictly less than `HealthCheckTimeout` in `internal/config/crossfield_test.go`
- [ ] T052 [P] [US3] Write a unit test asserting sensitive fields render as `***` and never appear in cleartext in `internal/config/mask_test.go`

### Implementation for User Story 3

- [ ] T053 [US3] Implement the `ValidationError` type accumulating one message per failing field in `internal/config/errors.go`
- [ ] T054 [US3] Implement per-field validation rules for every setting in the `contracts/configuration.md` table in `internal/config/validate.go` (depends on T053)
- [ ] T055 [US3] Implement the cross-field validation rule in `internal/config/validate.go` (depends on T054)
- [ ] T056 [P] [US3] Implement `Config.String()` masking sensitive fields in `internal/config/mask.go`
- [ ] T057 [US3] Wire validation failure to emit `config_validation_failed` and exit non-zero before any listener opens or client connects in `cmd/api/main.go` and `cmd/worker/main.go` (depends on T054, T056)

**Checkpoint**: A misconfigured service fails immediately and explains itself, with no partial
initialization.

---

## Phase 6: User Story 4 - Graceful Shutdown on a Termination Signal (Priority: P2)

**Goal**: Ordered shutdown that flips readiness first, drains in-flight work, bounds itself by a
grace period, and exits immediately on a repeated signal.

**Independent Test**: Send SIGTERM during an in-flight request and observe the log sequence and exit
status; send it twice and observe immediate exit; shorten the grace period and observe forced exit.

### Tests for User Story 4

- [ ] T058 [P] [US4] Write unit tests for the shutdown state machine covering normal completion, grace-period expiry, and repeated signals in `internal/lifecycle/shutdown_test.go`
- [ ] T059 [P] [US4] Write a unit test asserting readiness reports not-ready immediately once the shutting-down gate is set, without running dependency checks in `internal/health/shutdown_gate_test.go`
- [ ] T060 [P] [US4] Write a unit test asserting a signal arriving during startup aborts safely without leaving a half-initialized state in `internal/lifecycle/startup_abort_test.go`

### Implementation for User Story 4

- [ ] T061 [US4] Implement the root context via `signal.NotifyContext` for SIGINT and SIGTERM in `internal/lifecycle/shutdown.go`
- [ ] T062 [US4] Implement the grace-period bound with `shutdown_timeout` logging and forced exit in `internal/lifecycle/shutdown.go` (depends on T061)
- [ ] T063 [US4] Implement the second-signal immediate-exit path and safe startup abort in `internal/lifecycle/shutdown.go` (depends on T062)
- [ ] T064 [US4] Add the `ShuttingDown` gate that overrides dependency state unconditionally in `internal/health/aggregate.go`
- [ ] T065 [US4] Order shutdown as readiness gate first, then `http.Server.Shutdown`, then dependency client close in `internal/httpserver/server.go` (depends on T064)
- [ ] T066 [US4] Wire the shutdown sequence into both binaries with identical semantics in `cmd/api/main.go` and `cmd/worker/main.go` (depends on T063, T065)

**Checkpoint**: Both services drain predictably, which is the precondition for the F04/F05 restart
drills.

---

## Phase 7: User Story 5 - Trace a Single Request Through Structured Logs (Priority: P3)

**Goal**: Every log entry parses independently with all required fields, and a W3C-shaped
correlation ID ties one request's entries together.

**Independent Test**: Send a request with a `traceparent` header and confirm every resulting log
entry carries that trace ID; send one without and confirm a new 32-hex ID is generated.

### Tests for User Story 5

- [ ] T067 [P] [US5] Write a unit test asserting `trace_id` injection from context and the all-zero default when no correlation exists in `internal/logging/handler_test.go`
- [ ] T068 [P] [US5] Write a unit test asserting `traceparent` adoption and generation of a valid 32-hex ID when absent in `internal/logging/context_test.go`
- [ ] T069 [P] [US5] Write a unit test asserting entries below the configured level are not emitted in `internal/logging/logger_test.go`
- [ ] T070 [P] [US5] Write a unit test asserting every emitted record carries the required fields from `contracts/log-record.md` in `internal/logging/record_test.go`

### Implementation for User Story 5

- [ ] T071 [US5] Implement the unexported context key, the 16-byte ID generator, and the accessor in `internal/logging/context.go`
- [ ] T072 [US5] Implement `traceparent` header parsing and trace ID adoption in `internal/logging/context.go` (depends on T071)
- [ ] T073 [US5] Implement the context-aware `slog.Handler` that injects `trace_id` on every record in `internal/logging/handler.go` (depends on T071)
- [ ] T074 [US5] Add the correlation middleware populating the context on every inbound request in `internal/httpserver/middleware.go` (depends on T072)
- [ ] T075 [US5] Replace ad-hoc log messages with the stable event names and `error_class` vocabulary from `contracts/log-record.md` across `internal/health/`, `internal/config/`, `internal/lifecycle/`, and `internal/httpserver/`

**Checkpoint**: The log contract F08 will build on is fixed and enforced by tests.

---

## Phase 8: User Story 6 - Changes Are Verified Automatically on Submission (Priority: P3)

**Goal**: CI runs exactly what `make check` runs, blocks all three defect classes, and reports
within the SC-009 window.

**Independent Test**: Push a branch containing a compilation error, then one with a lint violation,
then one with a failing test, and confirm CI fails and names the offending item each time.

### Tests for User Story 6

- [ ] T076 [P] [US6] Write a test asserting `make check` and the CI workflow invoke the same underlying commands in `tests/integration/toolchain_test.go`

### Implementation for User Story 6

- [ ] T077 [US6] Expand the lint ruleset to cover the unused-result, shadowed-error, and unclosed-resource classes named in R-011 in `.golangci.yml`
- [ ] T078 [US6] Make `check` aggregate `go build`, `go vet`, `go tool golangci-lint run`, and unit tests as the single FR-036 command in `Makefile`
- [ ] T079 [US6] Add the lint job invoking `go tool golangci-lint run` rather than a separately versioned action in `.github/workflows/ci.yml` (depends on T077, T078)
- [ ] T080 [US6] Add the integration job that brings up compose before running the `integration` build tag in `.github/workflows/ci.yml` (depends on T079)
- [ ] T081 [US6] Verify the pipeline blocks a compilation error, a lint violation, and a failing test, and record the three run outcomes in `specs/001-platform-foundation/ci-verification.md`

**Checkpoint**: All six stories complete.

---

## Phase 9: Polish & Cross-Cutting Concerns

- [ ] T082 [P] Update the repository layout and development quickstart sections to match what was actually built in `README.md`
- [ ] T083 [P] Reconcile `.env.example` against the final `contracts/configuration.md` table in `.env.example`
- [ ] T084 Verify every registered metric name and label matches `contracts/metrics.md`, with no unbounded label values in `internal/observability/`
- [ ] T085 Verify readiness and liveness responses validate against the schemas in `contracts/health-api.yaml` in `tests/integration/contract_test.go`
- [ ] T086 Execute the V1–V8 validation scenarios in `quickstart.md` and record pass/fail for each
- [ ] T087 Record the observed SC-001 cold-start time, SC-009 CI duration, and SC-010 cycle results, including hardware and commands per Constitution Principle III, in `specs/001-platform-foundation/measurements.md`
- [ ] T088 Confirm no sensitive value appears in any log or HTTP response across the whole feature in `tests/integration/secrets_test.go`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on Setup — **blocks every user story**
- **US1 (Phase 3)** and **US2 (Phase 4)**: both depend only on Foundational. US2's T048 also touches
  `docker-compose.yml`, created in US1's T027, so if the two are worked in parallel T048 must land
  after T027
- **US3 (Phase 5)**: depends on Foundational (`internal/config` mechanism from T009–T011)
- **US4 (Phase 6)**: depends on Foundational; T064 edits `internal/health/aggregate.go`, so it must
  follow US2's T045
- **US5 (Phase 7)**: depends on Foundational; T074 edits `internal/httpserver/middleware.go`, so it
  must follow T018
- **US6 (Phase 8)**: depends on Setup for the workflow file; the integration job in T080 needs US1's
  compose stack
- **Polish (Phase 9)**: depends on every story being complete

### Cross-story file contention

These are the only files touched by more than one story. Sequence them as noted rather than editing
in parallel:

| File | Stories | Order |
| --- | --- | --- |
| `docker-compose.yml` | US1 (T024–T027), US2 (T048) | US1 first |
| `internal/health/aggregate.go` | US2 (T044, T045), US4 (T064) | US2 first |
| `internal/httpserver/middleware.go` | Foundational (T018), US5 (T074) | Foundational first |
| `cmd/api/main.go`, `cmd/worker/main.go` | Foundational, US2, US3, US4 | In phase order |
| `.github/workflows/ci.yml` | Setup (T005), US6 (T079, T080) | Setup first |
| `Makefile` | Setup (T004), US1 (T029, T032), US6 (T078) | In phase order |

### Within Each User Story

- Tests are written first and confirmed failing before implementation
- Entities and interfaces before the services that use them
- Services before handlers
- Handler before binary wiring

### Parallel Opportunities

- Setup: T003–T008 all parallel
- Foundational: T012, T013, T017, T019 parallel; T009→T010→T011 and T014→T016→T018 are chains
- US1: both tests parallel; T028, T030, T031 parallel with the compose chain T024→T027
- US2: all six tests parallel; the three checkers T041–T043 parallel after T040
- US3: all four tests parallel; T056 parallel with the T053→T054→T055 chain
- US4: all three tests parallel; implementation is largely a single-file chain
- US5: all four tests parallel; T073 parallel with T072
- Across stories: once Foundational is done, US3 and US5 can proceed alongside US1/US2 with the file
  contention above respected

---

## Parallel Example: User Story 2

```bash
# Write all six tests together, confirm they fail:
Task: "Unit tests for readiness aggregation across mixed states in internal/health/aggregate_test.go"
Task: "Unit test for readiness response schema and status mapping in internal/health/handler_test.go"
Task: "Unit test for bounded failure reason vocabulary in internal/health/checker_test.go"
Task: "Integration test for each probe against the live stack in tests/integration/deps_test.go"
Task: "Integration test for 503 with correct failing dependency in tests/integration/deps_down_test.go"
Task: "Integration test for starting with the stack down in tests/integration/startup_test.go"

# Then the three checkers together, after the Checker interface lands:
Task: "Kafka checker via kadm metadata in internal/health/kafka.go"
Task: "ClickHouse checker via native Ping in internal/health/clickhouse.go"
Task: "Redis checker via PING in internal/health/redis.go"
```

---

## Implementation Strategy

### MVP scope

The demonstrable MVP for this feature is **Phases 1–4** (Setup, Foundational, US1, US2): the stack
starts from clean with one command and reports accurate, dependency-aware health. That is what F02
and F03 need in order to begin, and it is the first point at which the feature can be shown to
someone.

US3–US6 harden that MVP and are prerequisites for later work rather than for a demo: US4 in
particular is the precondition for F05's restart drills, and US5 fixes the log contract that F08
depends on.

### Incremental delivery

1. Phase 1 + Phase 2 → both binaries start and serve liveness and metrics
2. + Phase 3 (US1) → one command brings up the whole stack, reproducibly
3. + Phase 4 (US2) → **MVP checkpoint**; stop and validate against `quickstart.md` V1, V2, V8
4. + Phase 5 (US3) → misconfiguration fails fast and explains itself
5. + Phase 6 (US4) → shutdown is predictable, unblocking F05's drills
6. + Phase 7 (US5) → log contract fixed, unblocking F08
7. + Phase 8 (US6) → CI blocks regressions
8. + Phase 9 → measurements recorded and contracts verified

### Parallel team strategy

With more than one developer, after Foundational completes: one takes US1+US2 (the MVP path), a
second takes US3+US5 (both pure-logic, low file contention), a third takes US4+US6. Respect the
cross-story file contention table.

---

## Notes

- `[P]` means a different file and no dependency on an incomplete task
- Every task names an exact file path; a task without one is under-specified
- Confirm each test fails before implementing against it (Constitution Principle II)
- Commit after each task or logical group
- Any number recorded in T087 must carry its hardware, command, and raw output
  (Constitution Principle III) — targets are never written as achieved results
