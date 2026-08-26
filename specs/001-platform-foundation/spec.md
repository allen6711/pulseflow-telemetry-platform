# Feature Specification: Platform Foundation & Local Development Environment

**Feature Branch**: `001-platform-foundation`

**Created**: 2026-08-26

**Status**: Draft

**Input**: User description: "Build the PulseFlow project skeleton and local development environment. This includes: a Go module with two runnable binaries, cmd/api and cmd/worker, both supporting graceful shutdown; internal/config providing environment-variable-driven configuration with sensible defaults and startup-time validation; a structured JSON logging package built on log/slog that reserves a trace_id field; a docker-compose.yml that starts Kafka, ClickHouse, Redis, and Prometheus with healthchecks and pinned image versions; an API exposing /v1/health/live as a pure liveness probe and /v1/health/ready as a dependency-aware readiness probe that checks Kafka, ClickHouse, and Redis and returns 503 listing the failing dependencies; a Makefile providing build, test, lint, and compose lifecycle commands; and a GitHub Actions workflow running build, go vet, and unit tests. This feature includes no business logic and no Kubernetes deployment."

## User Scenarios & Testing *(mandatory)*

The users of this feature are the **developer** and the **operator**. In this project they are the
same person, but their needs are described separately because they want different things from the
system.

What this feature delivers is a platform shell that starts, can be observed, and can be verified —
so that every later feature builds on top of it instead of re-inventing its own environment,
configuration, and health checking.

### User Story 1 - One Command Starts the Whole Local Environment (Priority: P1)

A developer who has just obtained the project on a clean machine runs a single command and gets
every supporting service the platform needs — event streaming, analytical storage, caching, and
metrics collection — plus this project's two service processes. They can immediately see whether
each service is ready. They do not install any supporting service by hand, and they do not start
things in a particular order.

**Why this priority**: without a repeatable environment, no later feature can be developed or
verified. This is the one starting point in the project with no prerequisites.

**Independent Test**: on a machine with no supporting services installed, clone the project, run
the start command, and observe whether all services reach a ready state. That alone fully tests
this story, and it delivers the standalone value of a working development environment.

**Acceptance Scenarios**:

1. **Given** a clean machine with only a container runtime and no supporting services installed, **When** the developer runs the single start command, **Then** event streaming, analytical storage, caching, and metrics collection all start, along with this project's ingestion and processing services, and all report ready.
2. **Given** the environment is running, **When** the developer inspects the health of each supporting service, **Then** each reports an explicit ready or not-ready state rather than merely "the container exists".
3. **Given** the environment is running, **When** the developer runs the stop command, **Then** all services stop and leave behind no residual state that would affect the next startup.
4. **Given** the developer runs the start command twice in a row, **When** the second startup completes, **Then** the result matches the first, with no failure caused by residual state.

---

### User Story 2 - Health Probes Distinguish "Process Alive" from "Ready for Traffic" (Priority: P1)

An operator needs two health signals with distinct meanings. One answers only "is this process
still alive?" and drives the decision to restart it. The other answers "can this process serve
correctly right now?" and drives the decision to route traffic to it. When a dependency becomes
unavailable, the second signal must turn not-ready and name the failing dependency, while the first
must keep succeeding so that no needless restart loop begins.

**Why this priority**: this underpins later container orchestration and every failure drill. If the
two signals blur together, a brief dependency outage restarts the service repeatedly and the
reliability tests stop meaning anything.

**Independent Test**: start the environment, stop one dependency at a time, and observe how the two
probes differ in their responses. That fully tests this story.

**Acceptance Scenarios**:

1. **Given** all dependencies are healthy, **When** the readiness probe is queried, **Then** it responds with a success status and lists each dependency as healthy.
2. **Given** the analytical storage service is stopped, **When** the readiness probe is queried, **Then** it responds with a service-unavailable status and the response body explicitly identifies analytical storage as the failing dependency.
3. **Given** the analytical storage service is stopped, **When** the liveness probe is queried, **Then** it still responds with success.
4. **Given** several dependencies are unavailable at once, **When** the readiness probe is queried, **Then** the response lists **all** failing dependencies, not just the first one encountered.
5. **Given** a dependency recovers, **When** the readiness probe is queried again, **Then** it returns to success within 10 seconds, with no service restart required.
6. **Given** a dependency is responding slowly, **When** the readiness probe is queried, **Then** the probe responds within its configured timeout rather than waiting indefinitely.

---

### User Story 3 - Configuration Errors Fail at Startup (Priority: P2)

A developer adjusts service behavior through environment variables — dependency addresses, listen
port, log level. Every setting has a usable default so local development needs no upfront variable
wrangling. But the moment an invalid value is supplied, the service must fail at startup and say
why, rather than starting successfully and misbehaving once it handles traffic.

**Why this priority**: a configuration error that only surfaces at runtime gets mistaken for a code
defect, and it introduces noise that is hard to trace during the later performance and reliability
experiments.

**Independent Test**: start the service three times — with valid configuration, with missing
configuration, and with invalid configuration — and observe the outcome and error message each
time.

**Acceptance Scenarios**:

1. **Given** no environment variables are supplied, **When** the service starts in a local development environment, **Then** it starts successfully using defaults.
2. **Given** a setting receives a malformed value (for example a non-numeric port), **When** the service starts, **Then** startup fails and the error message names the setting and explains why the value is invalid.
3. **Given** a setting receives a value outside the permitted range, **When** the service starts, **Then** startup fails and the error message states the permitted range.
4. **Given** configuration validation has failed, **When** the service state is inspected, **Then** the service has neither entered a traffic-accepting state nor partially initialized.
5. **Given** configuration contains a sensitive value, **When** the service emits its startup information, **Then** the sensitive value does not appear in cleartext in the logs.

---

### User Story 4 - Graceful Shutdown on a Termination Signal (Priority: P2)

When an operator terminates a service, it must stop accepting new work, complete or safely abandon
the work in hand, release its outbound connections, and then exit within a bounded grace period. If
it has not finished by the end of that period, it must force its own exit rather than hanging
indefinitely.

**Why this priority**: the later worker restart and rebalance experiments (F04, F05, F09) rest
entirely on termination being predictable. If shutdown behavior is uncertain, the results of those
experiments cannot be interpreted.

**Independent Test**: send a termination signal to the service and observe its shutdown sequence
and exit timing.

**Acceptance Scenarios**:

1. **Given** the ingestion service is handling requests, **When** a termination signal arrives, **Then** it stops accepting new requests but lets in-flight requests finish before exiting.
2. **Given** the service is shutting down, **When** the readiness probe is queried, **Then** it responds not-ready so that traffic stops being routed to it.
3. **Given** the service finishes cleanup within the grace period, **When** its exit is observed, **Then** it exits with a success status and emits a shutdown-complete log entry.
4. **Given** cleanup exceeds the grace period, **When** the grace period expires, **Then** the service forces its exit and records a "forced shutdown on timeout" event.
5. **Given** a termination signal is sent, **When** the processing service's behavior is observed, **Then** its shutdown sequence carries the same semantics as the ingestion service (stop taking new work → clean up → exit).

---

### User Story 5 - Trace a Single Request Through Structured Logs (Priority: P3)

Someone debugging needs to filter logs in a machine-parseable way and needs one correlation
identifier that ties together the log entries a single request produced across different
components. Even though this feature has no cross-process request flow yet, the log format and the
correlation identifier field must be fixed now so later components adopt them directly.

**Why this priority**: this is where the constitution's "Observability First" principle lands. A
format unified late forces every existing component to be rewritten. But this feature has no real
request chain yet, so it ranks below the first four stories.

**Independent Test**: trigger a number of events, feed the log output to a structured parsing tool,
and check field completeness.

**Acceptance Scenarios**:

1. **Given** the service is running, **When** any log entry is produced, **Then** that entry parses independently as a structured record containing timestamp, level, service name, version, message, and correlation identifier fields.
2. **Given** a request carrying a correlation identifier enters the service, **When** that request produces several log entries, **Then** every related entry carries the same correlation identifier.
3. **Given** a request arrives without a correlation identifier, **When** the service handles it, **Then** the service generates a new identifier and uses it for all of that request's logs.
4. **Given** the configured log level is warning, **When** an info-level entry is produced, **Then** that entry is not emitted.
5. **Given** an error occurs, **When** the error log is inspected, **Then** that entry contains an error classification and enough context fields to locate the problem.

---

### User Story 6 - Changes Are Verified Automatically on Submission (Priority: P3)

When a contributor submits a change, an automated pipeline compiles the project, runs static
analysis and unit tests, and reports the outcome before merge. The contributor does not have to
memorize the full checklist locally, and the same checks are reproducible locally through a single
command.

**Why this priority**: it protects the quality baseline for later features, but does not affect what
this feature itself delivers.

**Independent Test**: submit a change that deliberately contains a static analysis violation and
observe whether the pipeline blocks it.

**Acceptance Scenarios**:

1. **Given** a change that passes every check, **When** it is submitted, **Then** the automated pipeline reports success.
2. **Given** a change that does not compile, **When** it is submitted, **Then** the pipeline reports failure and identifies the compilation error.
3. **Given** a change containing a static analysis violation, **When** it is submitted, **Then** the pipeline reports failure and identifies the location of the violation.
4. **Given** a change containing a failing unit test, **When** it is submitted, **Then** the pipeline reports failure and identifies the failing test.
5. **Given** a developer working locally, **When** they run the local check command, **Then** the checks executed match those run by the pipeline.

---

### Edge Cases

- A supporting service's port is already occupied by another process at startup → startup must fail
  explicitly and name the conflicting port rather than silently degrading.
- All supporting services are unavailable at once → the readiness probe must still respond (rather
  than failing itself) and must list every failing dependency.
- A dependency recovers or drops mid-probe → the probe reports the result of that particular check;
  it must not hang or return an inconsistent partial result.
- The readiness probe is queried at high frequency → checking dependencies must not impose
  additional load on them (result caching or a minimum re-check interval is required).
- Dependencies are not yet ready when the service starts (container startup race) → the service
  itself must still start successfully and report not-ready, rather than failing to start, and must
  become ready automatically once dependencies are up (FR-037).
- A termination signal arrives before startup has completed → the service must safely abort startup
  and exit, leaving no half-initialized state.
- Two termination signals arrive in succession → the second must trigger immediate exit rather than
  running the cleanup sequence again.
- Log content contains newlines or quotation marks → the structured format must remain parseable.
- Insufficient disk or memory prevents a supporting service from starting → the start command must
  report a clear error rather than waiting indefinitely.

## Requirements *(mandatory)*

### Functional Requirements

**Environment and Lifecycle**

- **FR-001**: The system MUST provide a single command that starts every supporting service required for local development (event streaming, analytical storage, caching, metrics collection) together with this project's ingestion and processing services.
- **FR-002**: Every supporting service MUST define a health check so that the startup flow can distinguish "container created" from "service available".
- **FR-003**: All supporting services MUST start at pinned versions. Floating version tags that change over time MUST NOT be used.
- **FR-004**: The system MUST provide a corresponding stop command, and repeated execution of the start command MUST produce a consistent result.
- **FR-005**: The system MUST provide a single command entry point for build, test, static analysis, and environment lifecycle operations.

**Health Probes**

- **FR-006**: The ingestion service MUST expose a liveness probe endpoint at `/v1/health/live` whose result MUST reflect only whether the process itself is alive, and MUST NOT fail because any external dependency is unavailable.
- **FR-007**: The ingestion service MUST expose a readiness probe endpoint at `/v1/health/ready` whose result MUST reflect the availability of the event streaming, analytical storage, and caching dependencies.
- **FR-008**: When any dependency is unavailable, the readiness probe MUST respond with a service-unavailable status code, and the response body MUST list **every** dependency by name along with its individual status.
- **FR-009**: The readiness probe's dependency checks MUST have a timeout bound, and a timeout MUST be treated as that dependency being unavailable.
- **FR-010**: The readiness probe MUST apply a minimum re-check interval to dependency check results so that high-frequency queries do not proportionally amplify load on the dependencies.
- **FR-011**: Once a dependency recovers, the readiness probe MUST return to success within the minimum re-check interval, and MUST NOT require a service restart.
- **FR-012**: The readiness probe response MUST NOT contain connection strings, credentials, or other sensitive configuration values.
- **FR-013**: The processing service MUST expose liveness and readiness probes with the same semantics as the ingestion service.

**Configuration Management**

- **FR-014**: The system MUST source configuration from environment variables, and every setting MUST have a default usable for local development.
- **FR-015**: The system MUST validate all configuration at startup, and validation failure MUST cause startup to fail.
- **FR-016**: A configuration validation error message MUST identify which setting failed, why the value it received is invalid, and the permitted format or range.
- **FR-017**: The system MUST NOT enter a partially initialized or traffic-accepting state when configuration validation fails.
- **FR-018**: The system MUST mask sensitive values when emitting configuration information.

**Graceful Shutdown**

- **FR-019**: Both services MUST respond to a termination signal and shut down in the order: stop accepting new work → complete or safely abandon in-flight work → release resources → exit.
- **FR-020**: Once a service enters its shutdown sequence, the readiness probe MUST immediately report not-ready.
- **FR-021**: The shutdown sequence MUST have a configurable grace period bound, and exceeding it MUST force an exit and record a timeout event.
- **FR-022**: Services MUST safely handle a termination signal that arrives before startup has completed.
- **FR-023**: A repeated termination signal MUST trigger an immediate exit, and MUST NOT re-run the cleanup sequence.

**Structured Logging**

- **FR-024**: All logs MUST be emitted in a structured format where each entry parses independently.
- **FR-025**: Every log entry MUST contain timestamp, severity level, service name, service version, message, and correlation identifier fields.
- **FR-026**: The system MUST provide a mechanism for propagating a correlation identifier through the lifetime of a single request or unit of work, so that all logs from that unit carry the same identifier.
- **FR-027**: When an incoming request carries no correlation identifier, the system MUST generate one.
- **FR-028**: The log level MUST be adjustable through configuration, and entries below the configured level MUST NOT be emitted.
- **FR-029**: Error logs MUST contain an error classification and enough context fields to locate the problem.
- **FR-030**: Logs MUST NOT emit sensitive configuration values.

**Metrics Foundation**

- **FR-031**: Both services MUST each expose a `/metrics` endpoint as the shared outlet through which later features register their metrics.
- **FR-032**: This feature MUST establish the shared metric registration mechanism and emit at minimum service build information and process runtime metrics. Business metrics are out of scope for this feature.
- **FR-033**: The metrics collection service MUST be configured to scrape the `/metrics` endpoints of both services automatically.

**Automated Verification**

- **FR-034**: The system MUST automatically run build, static analysis, and unit tests when a change is submitted.
- **FR-035**: Failure of any automated check MUST cause that verification run to report failure and identify the failing item and its location.
- **FR-036**: The checks run by the automated pipeline MUST be fully reproducible by a developer locally through a single command.

**Startup Resilience**

- **FR-037**: When one or more dependencies are unavailable at startup, a service MUST still start successfully and report not-ready, MUST NOT exit, and MUST become ready automatically once the dependencies are available, without operator intervention.

### Key Entities

- **Service Configuration**: a set of named settings, each with a name, a default, a permitted
  format or range, and a flag marking whether it is sensitive. Validated as a whole at startup;
  once validated it becomes immutable runtime configuration.
- **Dependency Health Status**: the check result for a single external dependency, containing the
  dependency name, whether it is healthy, how long the check took, and a failure reason
  classification when unhealthy.
- **Readiness Result**: the overall conclusion of one readiness query, aggregated from all
  dependency health statuses. Contains the overall status and the per-dependency breakdown, plus
  the time the result was produced so that the minimum re-check interval can be applied.
- **Log Record**: one structured event containing timestamp, severity level, service name, service
  version, message, correlation identifier, and optional error classification and context fields.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer with no prior exposure to this project, on a machine equipped only with a
  container runtime, can bring every service to a ready state within 10 minutes using nothing but
  the project documentation and a single command, with no manual intervention and no hand-installed
  supporting services.
- **SC-002**: Stopping any one of the three dependencies turns the readiness signal not-ready within
  10 seconds, and its response correctly identifies the failing dependency. All four combinations
  (three single failures, one total failure) pass at a rate of 100%.
- **SC-003**: With any or all external dependencies unavailable, the liveness signal keeps
  succeeding, with 0 false negatives.
- **SC-004**: After a dependency recovers, the readiness signal returns to success automatically
  within 10 seconds, with no service restart during the process.
- **SC-005**: For the three classes of configuration error — missing required value, malformed
  value, and value outside the permitted range — the startup-time interception rate is 100%, and
  each error message lets a reader determine which setting to change without consulting the source
  code.
- **SC-006**: Services complete shutdown within the configured grace period 100% of the time; no
  request arriving during shutdown is cut off without a response.
- **SC-007**: Across a random sample of 100 log entries, 100% parse as structured records and 100%
  carry all required fields (timestamp, level, service name, version, message, correlation
  identifier).
- **SC-008**: The logs produced by a single request can be retrieved completely using the
  correlation identifier alone, with a miss rate of 0.
- **SC-009**: The automated verification pipeline reports its result within 5 minutes of
  submission, and intercepts each of the three deliberately injected problem classes — compilation
  error, static analysis violation, and failing test — at a rate of 100%.
- **SC-010**: Running the start and stop commands five times consecutively produces a consistent
  result each time, with no failure caused by residual state.

## Assumptions

The following are the reasonable defaults chosen where the description did not specify. Departing
from any of them during implementation requires justification at the planning stage.

**Technology Choices (fixed by the project constitution, not decisions made by this feature)**

- Services are written in Go. Event streaming uses Kafka, analytical storage uses ClickHouse,
  caching uses Redis, metrics collection uses Prometheus, the local environment is orchestrated with
  Docker Compose, and automated verification uses GitHub Actions. These choices come from the
  "Technology & Scope Constraints" section of `.specify/memory/constitution.md` and are not
  re-evaluated here.

**Behavioral Defaults**

- Readiness dependency checks are lightweight connection-level checks (confirming a connection can
  be established and answered); they do not write or read data.
- The readiness dependency check timeout defaults to 2 seconds and the minimum result re-check
  interval defaults to 1 second. Both are configurable.
- The shutdown grace period defaults to 30 seconds and is configurable. It needs to exceed the
  expected processing time of a single request.
- If dependencies are not ready when a service starts, the service still starts successfully and
  reports not-ready rather than failing to start, tolerating container startup ordering races.
- The correlation identifier uses a format compatible with the distributed tracing introduced later,
  so that F08 can adopt tracing without changing the semantics of the log field.
- Logs are written to standard output by default, with collection left to the container runtime.
  Log retention and forwarding are not handled by this feature.
- This feature creates no persistent table schema. The analytical storage service only needs to
  start and accept connections.
- In this feature the processing service consumes no messages. It only needs startup, health probes,
  configuration, logging, and shutdown.

**Scope Boundaries**

- No business logic: no ingestion endpoint, no message consumption, no data writes or queries.
- No Kubernetes deployment configuration (that is F11).
- No business metrics and no distributed tracing (that is F08). This feature only establishes the
  metrics outlet and the registration mechanism.
- No authentication, authorization, or rate limiting.
- No production deployment, secret management, or TLS termination.
- Integration testing in this feature covers only the "the service can connect to its supporting
  services" level. Cross-boundary data correctness testing belongs to the features that own it.

## Dependencies

- No prerequisite features. This is the root node of the dependency graph in `FEATURES.md`.
- Later features F02 (Event Ingestion API), F03 (ClickHouse Analytical Store), and F08
  (Observability) all build on the configuration, logging, health probe, and metric registration
  mechanisms this feature provides.
- Requires the developer's machine to have a container runtime. Installing that runtime is not this
  feature's responsibility, but the documentation must state the required version.
