<!--
Sync Impact Report
==================
Version change: 1.0.0 → 1.0.1
Bump rationale: PATCH. The governing document was translated from Traditional Chinese to English
  for consistency with README.md and FEATURES.md. No principle was added, removed, or
  semantically changed; every MUST/MUST NOT/SHOULD retains its original force and scope.

Modified principles: none (translation only)
  I.   Observability First
  II.  Layered Testing — NON-NEGOTIABLE
  III. Reproducible Measurement
  IV.  At-Least-Once Delivery & Application-Level Idempotency
  V.   Simplicity & Scope Discipline

Added sections: none
Removed sections: none

Templates requiring updates:
  .specify/templates/plan-template.md — OK, no change needed; its "Constitution Check" section
    reads this file at runtime and can cite each principle's "Gate" bullet directly.
  .specify/templates/spec-template.md — OK, no change needed.
  .specify/templates/tasks-template.md — OK, no change needed.
  .specify/templates/checklist-template.md — OK, no change needed.

Follow-up TODOs: none

Prior history
-------------
1.0.0 (2026-08-24) — Initial ratification. All template placeholders replaced with concrete
  governance content.
-->

# PulseFlow Constitution

PulseFlow is a distributed telemetry ingestion and analytics platform. This constitution defines
the non-negotiable rules that every feature specification, plan, and implementation must follow.
It takes precedence over individual preference and prior habit.

## Core Principles

### I. Observability First

Any processing path that is added or modified — HTTP handler, Kafka producer or consumer, storage
write, cache access — MUST gain its observability in the same change. It MUST NOT be deferred:

- MUST emit at least one Prometheus metric covering whichever of traffic, latency, and failure
  applies to that path.
- MUST emit structured JSON logs carrying `trace_id`. Error logs MUST include an error
  classification and enough context to locate the problem.
- Metric labels MUST be bounded in cardinality. Using `event_id`, raw `tags` values, or any
  unbounded user-controlled string as a label is forbidden.
- When crossing a process boundary (HTTP → Kafka → worker), trace context MUST be propagated
  through message headers.
- Failure paths — retries, degradation, dead-letter — MUST carry metrics and logs to the same
  standard as success paths.

**Gate**: every new processing path in a plan maps to a named metric and a named log event.
Anything that cannot be mapped is a violation.

**Rationale**: this project's core value proposition is a distributed system that can be measured
and debugged. Instrumentation added after the fact reliably misses the failure paths, which are
exactly the paths that need to be observed.

### II. Layered Testing — NON-NEGOTIABLE

The testing strategy is layered by whether a process boundary is crossed, and each layer is
mandatory:

- **Pure logic MUST be unit tested**: schema validation, cache key generation, retry
  classification, query parameter validation, API error mapping. This logic MUST be testable
  without starting any external dependency.
- **Behavior crossing a Kafka / ClickHouse / Redis boundary MUST have integration tests** running
  against real services (testcontainers or docker compose). Substituting mocks for correctness
  verification at these boundaries is forbidden.
- **End-to-end behavior MUST be verified against the full container stack**, exercising the
  ingest → process → query chain plus the dead-letter, duplicate-event, and cache-hit branches.
- Fixing a defect MUST begin with a test that reproduces it.
- Behavior involving a correctness claim (aggregate values, deduplication, no loss) MUST assert
  expected values against a known fixture. Asserting merely that no error occurred is insufficient.

**Gate**: every feature's task list contains test tasks for the layers it touches. A feature that
crosses an external dependency but ships only unit tests is a violation.

**Rationale**: defects in distributed systems almost always live at process boundaries. A test
suite that passes on mocks provides zero coverage of that class of defect.

### III. Reproducible Measurement

Every performance and reliability number MUST be reproducible by someone else:

- Performance numbers MUST come from scripts and configuration committed to version control, never
  from a one-off command typed by hand.
- Publishing any measurement MUST include: hardware specification, command executed, dataset and
  how it was generated, full configuration, run duration, and raw output.
- Target values and measured results MUST be visually distinct in documentation. The suggested
  targets in the README MUST NOT be written as achieved outcomes before they are measured.
- Comparative experiments (cache on/off, worker count scaling) MUST hold every condition constant
  except the variable under test, and MUST record what that variable was.
- Reliability claims ("no events lost across restarts") MUST state trial count and success count
  rather than a qualitative description.

**Gate**: every number appearing in a spec, plan, or document traces back to a committed script and
a raw output file.

**Rationale**: performance numbers that cannot be reproduced carry no weight under technical
scrutiny, and they easily become misrepresentation without anyone intending it.

### IV. At-Least-Once Delivery & Application-Level Idempotency

The correctness semantics for event processing are at-least-once delivery plus application-level
idempotency. Implementations MUST observe the following:

- Kafka offsets MUST be committed only after the corresponding events have been persisted.
  Committing before processing is forbidden.
- The same `event_id` entering the system twice MUST NOT produce a second valid analytical record,
  and this property MUST be covered by a test.
- The ingestion API MUST return success (`202`) only after Kafka has acknowledged the write. When
  Kafka is unavailable it MUST return an error; returning success is forbidden.
- Processing failures MUST be explicitly classified as transient or permanent. Transient failures
  MUST be retried with bounded backoff; permanent failures and those exceeding the retry limit
  MUST be routed to a dead-letter topic.
- Dead-letter records MUST retain the original payload and enough metadata to determine the cause
  of failure, and MUST be inspectable.
- On receiving a termination signal, a worker MUST bring its current batch to a safe state —
  completing or abandoning it — before exiting, and events already acknowledged by Kafka MUST NOT
  be lost in the process.

**Gate**: every write to an external system in a plan can state its failure classification, its
offset behavior, and the basis for its idempotency.

**Rationale**: this is the project's central technical claim. A violation anywhere invalidates the
"no loss, no duplicates" claim everywhere.

### V. Simplicity & Scope Discipline

Architectural complexity MUST be justified, and MUST NOT exceed the project's declared scope:

- MUST NOT implement Kafka, ClickHouse, Redis, or any consensus algorithm (Raft/Paxos) from
  scratch.
- MUST NOT introduce a service mesh, multi-region active-active deployment, or a split into dozens
  of microservices.
- MUST NOT build a full commercial monitoring UI, and MUST NOT add AI/LLM features.
- The project MUST stay small enough that a reviewer can understand its architecture quickly. Any
  increase in component count MUST be justified in the plan.
- A new abstraction, interface, or layer of indirection MUST correspond to a concrete need that
  exists today, never to hypothetical future flexibility.
- Stretch goals MUST NOT begin until every MVP acceptance criterion passes.

**Gate**: every new component, package, or abstraction in a plan names the concrete problem it
solves today. Anything that cannot is recorded in Complexity Tracking along with why the simpler
alternative was rejected.

**Rationale**: the purpose of this project is to demonstrate distributed processing clearly.
Over-engineering dilutes both the demonstration and the maintainability.

## Technology & Scope Constraints

- **Language and runtime**: backend services are written in Go. Shared logic lives under
  `internal/`; binaries live under `cmd/api` and `cmd/worker`.
- **Fixed technology choices**: Kafka for event streaming, ClickHouse for analytical storage, Redis
  for caching, OpenTelemetry and Prometheus for observability, Docker and Docker Compose for
  containerization, Kubernetes for orchestration, k6 for load testing, GitHub Actions for CI.
  Replacing any of these requires amending this constitution.
- **Configuration**: all mutable configuration MUST come from environment variables, MUST have
  defaults, and MUST be validated at service startup. A configuration error MUST cause startup to
  fail rather than surfacing at runtime.
- **API contract**: external HTTP endpoints are prefixed with `/v1`. A breaking change to a
  published endpoint MUST bump this constitution's MAJOR version. Error responses MUST use a
  uniform JSON structure.
- **Data schema**: the telemetry event schema MUST carry a version field. Workers MUST validate
  the version and reject versions they cannot process.
- **Health checks**: the liveness probe MUST reflect only process liveness. The readiness probe
  MUST reflect dependency availability and MUST name which dependency failed.
- **Local environment**: `docker compose up` MUST start every dependency the demo needs, and each
  dependency MUST define a healthcheck.

## Development Workflow & Quality Gates

- **Feature workflow**: each feature runs `/speckit.specify` → `/speckit.clarify` (when design
  trade-offs are unresolved) → `/speckit.plan` → `/speckit.tasks` → `/speckit.analyze` →
  `/speckit.implement`, in that order. Feature decomposition and dependency order are governed by
  `FEATURES.md`.
- **Constitution gate**: the Constitution Check produced by `/speckit.plan` MUST be evaluated
  against all five principles above, one by one. Any violation MUST be recorded in that plan's
  Complexity Tracking table with an explanation of why the simpler alternative is unworkable. A
  violation that cannot be explained MUST be designed away rather than waved through.
- **Dependency direction**: a later feature MUST NOT break the external contract of an earlier one.
  When modifying an existing component is unavoidable (for example, centralizing instrumentation),
  that feature's spec MUST list the affected existing files and the scope of the change.
- **CI gate**: every pull request MUST pass build, `go vet`, lint, and unit tests. Features that
  touch an external dependency MUST run their integration tests in CI, with a reduced dataset where
  necessary.
- **Merge condition**: a feature may merge once all of its acceptance criteria are verifiably met
  and its testing-layer obligations are satisfied.
- **Documentation sync**: changes to an external API, a metric name, or the deployment method MUST
  update the corresponding documentation in the same change.

## Governance

- **Standing**: this constitution supersedes all other development practices and personal
  preferences. Existing practice that conflicts with it MUST be corrected.
- **Amendment procedure**: amendments MUST be proposed as a change to this file, and that change
  MUST include: (a) the amendment itself, (b) an updated Sync Impact Report, (c) the version
  adjustment, and (d) a plan for handling affected existing code — either fixed immediately or
  recorded as explicit technical debt.
- **Versioning policy** (semantic versioning):
  - **MAJOR**: removing a principle, redefining an existing principle in a backward-incompatible
    way, or changing a fixed technology choice or a published API contract.
  - **MINOR**: adding a principle or section, or materially expanding existing guidance.
  - **PATCH**: wording clarifications, typo fixes, translations, and other changes that do not
    alter meaning.
- **Compliance review**: all pull requests and code reviews MUST verify compliance with this
  constitution. At each milestone completion (see M1–M6 in `FEATURES.md`), the constitution MUST be
  reviewed to confirm it still reflects the state of the project.
- **Exceptions**: an exception to a principle MUST be recorded in the corresponding feature's plan,
  including the specific justification, the blast radius, and the condition under which the
  exception is removed. An unrecorded exception is treated as a defect.
- **Runtime guidance**: project scope and requirements are governed by `README.md`; feature
  decomposition and dependency order are governed by `FEATURES.md`. Neither MUST conflict with this
  constitution; where they do, this constitution prevails.

**Version**: 1.0.1 | **Ratified**: 2026-08-24 | **Last Amended**: 2026-08-26
