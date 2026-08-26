# Quickstart & Validation Guide: Platform Foundation

**Feature**: `001-platform-foundation` | **Date**: 2026-08-26

How to run this feature and prove it satisfies its specification. Every section maps to numbered
requirements and success criteria in [spec.md](./spec.md). Field-level details live in
[contracts/](./contracts/) and are referenced rather than repeated.

## Prerequisites

| Tool | Minimum | Observed on the development machine |
| --- | --- | --- |
| Go | 1.25 | 1.26.5 (darwin/arm64) |
| Docker Engine | 25 | 29.3.1 |
| Docker Compose | v2 | v5.1.1 |

No other installation is required. Lint tooling is pinned in `go.mod` via the `tool` directive and
fetched on first use, so there is nothing to install globally (R-011).

## Pinned image versions

Every image is pinned to an explicit patch tag (FR-003). Upgrading one is a deliberate, reviewable
change, because a benchmark measured against a floating tag is not reproducible later.

| Service | Image | Local port |
| --- | --- | --- |
| Kafka (KRaft, single broker) | `apache/kafka:4.1.0` | 9092 |
| ClickHouse | `clickhouse/clickhouse-server:25.8` | 9000 (native), 8123 (HTTP) |
| Redis | `redis:8.2-alpine` | 6379 |
| Prometheus | `prom/prometheus:v3.6.0` | 9090 |
| PulseFlow API | built locally | 8080 |
| PulseFlow worker | built locally | 8081 |

## Start the stack

```bash
make up          # docker compose up -d --build, waits for healthy
make ps          # show per-service health
make logs        # follow application logs
make down        # stop and remove
```

`make up` is the single command referenced by FR-001 and SC-001. It returns only once every
supporting service reports healthy through its own protocol-level healthcheck (FR-002, R-013), so
startup is deterministic rather than racy.

## Development commands

```bash
make build              # build both binaries
make test               # unit tests only, no dependencies needed
make test-integration   # //go:build integration, requires `make up` first
make lint               # go vet + golangci-lint via the pinned tool directive
make check              # everything CI runs — the FR-036 single command
```

`make check` runs exactly what `.github/workflows/ci.yml` runs. That is the point: a contributor
cannot pass locally and fail in CI on a different rule set.

---

## Validation scenarios

### V1 — One command starts everything (US1 · FR-001, FR-002, FR-004 · SC-001)

```bash
make down && docker system prune -f     # simulate a clean machine
time make up
make ps
```

**Expected**: every service reaches `healthy`; both application containers stay up. Record the
elapsed time against SC-001's 10-minute budget. `scripts/verify-cold-start.sh` automates this and
writes its measurement to a file — per constitution Principle III, the number is recorded when
measured, not asserted in advance.

Then repeat, to confirm no residual state (FR-004, SC-010):

```bash
scripts/verify-restart-idempotence.sh    # 5 consecutive up/down cycles
```

### V2 — Liveness and readiness mean different things (US2 · FR-006 … FR-013 · SC-002, SC-003)

Healthy baseline:

```bash
curl -s localhost:8080/v1/health/ready | jq
```

**Expected**: `200`, `status: "ready"`, and a `dependencies` array with all three entries healthy.
Schema: [contracts/health-api.yaml](./contracts/health-api.yaml).

Stop one dependency:

```bash
docker compose stop clickhouse
sleep 2
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/v1/health/live    # expect 200
curl -s -w '\n%{http_code}\n' localhost:8080/v1/health/ready | jq         # expect 503
```

**Expected**: liveness stays `200` (FR-006, SC-003) — this is the check that prevents a dependency
outage from causing a restart loop. Readiness returns `503`, names `clickhouse` as unhealthy with a
`reason`, and **still lists kafka and redis** with their own statuses (FR-008).

Verify the response leaks nothing (FR-012): the failing entry carries a classified `reason` such as
`timeout`, never a driver error containing a host, port, or credential. The full error appears in
the logs instead, under `dependency_check_failed`.

Stop everything, then recover:

```bash
docker compose stop kafka redis
curl -s localhost:8080/v1/health/ready | jq '.dependencies'   # all three listed, all unhealthy
docker compose start kafka clickhouse redis
sleep 3
curl -s localhost:8080/v1/health/ready | jq '.status'         # back to "ready", no restart
```

**Expected**: recovery within 10 seconds with no service restart (FR-011, SC-004).

Verify probe load is bounded (FR-010):

```bash
for i in $(seq 1 100); do curl -s -o /dev/null localhost:8080/v1/health/ready; done
curl -s localhost:8080/metrics | grep pulseflow_dependency_check_duration_seconds_count
```

**Expected**: the check count grows by roughly the elapsed seconds divided by
`PULSEFLOW_HEALTH_CACHE_TTL`, not by 100. Without the cache and `singleflight` collapsing,
100 probes would mean 300 dependency round trips.

### V3 — Configuration fails fast and explains itself (US3 · FR-014 … FR-018 · SC-005)

```bash
go run ./cmd/api                                        # no env set: starts on defaults
PULSEFLOW_HTTP_PORT=http PULSEFLOW_LOG_LEVEL=verbose go run ./cmd/api
```

**Expected**: the second exits non-zero without opening a listener, and reports **both** errors at
once — not the first one only. Each line names the variable, quotes the value received, and states
the permitted range. Format: [contracts/configuration.md](./contracts/configuration.md).

The one-at-a-time alternative is what SC-005 exists to rule out: three mistakes should cost one
restart, not three.

Cross-field validation:

```bash
PULSEFLOW_HEALTH_CACHE_TTL=5s PULSEFLOW_HEALTH_CHECK_TIMEOUT=2s go run ./cmd/api
```

**Expected**: rejected. A cached failure must not outlive the check that produced it, or FR-011's
recovery window becomes unbounded.

Sensitive masking (FR-018):

```bash
PULSEFLOW_CLICKHOUSE_PASSWORD=hunter2 go run ./cmd/api 2>&1 | grep -c hunter2
```

**Expected**: `0`.

### V4 — Graceful shutdown (US4 · FR-019 … FR-023 · SC-006)

```bash
go run ./cmd/api &
PID=$!
curl -s "localhost:8080/v1/health/ready" &      # in-flight request
kill -TERM $PID
```

**Expected log sequence**: `shutdown_started` → in-flight request completes → `shutdown_complete`,
exit status 0. While shutting down, `/v1/health/ready` reports `not_ready` **before** connections
are closed (FR-020) — flipping readiness after shutdown begins would race the load balancer and drop
requests.

Second signal (FR-023):

```bash
kill -TERM $PID; kill -TERM $PID
```

**Expected**: immediate exit, no second cleanup pass.

Grace period expiry (FR-021):

```bash
PULSEFLOW_SHUTDOWN_GRACE_PERIOD=1s go run ./cmd/api    # then send SIGTERM during a slow request
```

**Expected**: `shutdown_timeout` logged and forced exit — never an indefinite hang.

### V5 — Structured logs and correlation (US5 · FR-024 … FR-030 · SC-007, SC-008)

```bash
make logs | jq -c 'select(.trace_id)' | head
```

**Expected**: every line parses as JSON and carries `time`, `level`, `msg`, `service`, `version`,
`trace_id` (SC-007). Field contract: [contracts/log-record.md](./contracts/log-record.md).

Correlation across a request (FR-026, FR-027, SC-008):

```bash
curl -s -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01' \
  localhost:8080/v1/health/ready > /dev/null
make logs | jq -c 'select(.trace_id=="4bf92f3577b34da6a3ce929d0e0e4736")'
```

**Expected**: the supplied trace ID is adopted, and every log line from that request carries it. A
request without a `traceparent` gets a freshly generated 32-hex-character ID in the same format —
which is what lets F08 introduce OpenTelemetry without changing this contract.

Level filtering (FR-028):

```bash
PULSEFLOW_LOG_LEVEL=warn go run ./cmd/api 2>&1 | jq -c 'select(.level=="INFO")'
```

**Expected**: no output.

### V6 — Metrics endpoint (FR-031 … FR-033)

```bash
curl -s localhost:8080/metrics | grep -E '^pulseflow_'
curl -s localhost:8081/metrics | grep -E '^pulseflow_'    # worker exposes its own
open http://localhost:9090/targets                        # both targets UP
```

**Expected**: `pulseflow_build_info`, `pulseflow_http_requests_total`,
`pulseflow_http_request_duration_seconds`, `pulseflow_dependency_up`,
`pulseflow_dependency_check_duration_seconds`, plus `go_*` and `process_*`. Full list and label
rules: [contracts/metrics.md](./contracts/metrics.md).

Confirm label cardinality is bounded: `route` values are registered patterns, and `dependency` takes
only `kafka`, `clickhouse`, `redis`.

### V7 — Automated verification (US6 · FR-034 … FR-036 · SC-009)

```bash
make check      # identical to CI
```

Confirm the pipeline actually blocks by pushing a branch with each defect class in turn — a
compilation error, a lint violation, and a failing unit test — and checking that CI fails and names
the offending item (SC-009).

### V8 — Startup with dependencies absent (edge case · FR-037)

```bash
make down
go run ./cmd/api &
curl -s -w '\n%{http_code}\n' localhost:8080/v1/health/ready | jq
```

**Expected**: the service **starts successfully** and reports `503 not_ready` listing all three
dependencies as unhealthy. It does not exit.

This is the container startup race, and getting it wrong is the difference between a stack that
comes up on the first try and one that CrashLoopBackOffs in F11. `docker compose` uses
`depends_on: service_healthy` for developer convenience, but correctness must not rely on ordering.

```bash
make up
sleep 3
curl -s localhost:8080/v1/health/ready | jq '.status'    # becomes "ready" on its own
```

---

## Definition of done

| # | Check | Requirements |
| --- | --- | --- |
| 1 | `make up` brings every service healthy from clean | FR-001–004 · SC-001 |
| 2 | Readiness names all failing dependencies; liveness unaffected | FR-006–013 · SC-002, SC-003 |
| 3 | Recovery is automatic without restart | FR-011 · SC-004 |
| 4 | Probe frequency does not amplify dependency load | FR-010 |
| 5 | All configuration errors reported at once, with usable messages | FR-014–018 · SC-005 |
| 6 | Shutdown drains, flips readiness first, and bounds itself | FR-019–023 · SC-006 |
| 7 | Every log line parses, with all required fields | FR-024–030 · SC-007, SC-008 |
| 8 | Both services expose `/metrics`; Prometheus scrapes both | FR-031–033 |
| 9 | `make check` matches CI and blocks all three defect classes | FR-034–036 · SC-009 |
| 10 | Services start with dependencies down and self-heal | FR-037 · FR-011 |
| 11 | Unit tests pass with no dependencies; integration tests pass against compose | Constitution II |
