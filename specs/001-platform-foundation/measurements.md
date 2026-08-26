# Measurements (T092)

**Feature**: `001-platform-foundation`

Constitution Principle III: every number here comes from a committed script, and
is recorded with the conditions that produced it. Values are filled in when the
measurement is actually taken. A target is never written as if it had been
measured, so the result columns stay empty until then.

## SC-001 — Cold start to all services healthy

**Budget**: 10 minutes, single command, no manual intervention
**Script**: `scripts/verify-cold-start.sh`
**Raw output**: `benchmarks/raw/cold-start-<timestamp>.txt`

| Date (UTC) | Commit | Host | Elapsed | Within budget |
| --- | --- | --- | --- | --- |
| | | | | |

## SC-010 — Repeated start/stop consistency

**Budget**: 5 consecutive cycles, identical result each time
**Script**: `scripts/verify-restart-idempotence.sh`
**Raw output**: `benchmarks/raw/restart-idempotence-<timestamp>.txt`

| Date (UTC) | Commit | Cycles run | Cycles failed |
| --- | --- | --- | --- |
| | | | |

## SC-009 — Automated verification feedback time

**Budget**: result within 5 minutes of submission
**Source**: GitHub Actions run duration for `.github/workflows/ci.yml`
**Interception record**: [ci-verification.md](./ci-verification.md)

| Date (UTC) | Commit | Build+test | Lint | Integration | Total wall clock |
| --- | --- | --- | --- | --- | --- |
| | | | | | |

## Host template

Copy this block into each result row's notes so the numbers stay reproducible:

```text
uname:
cpus:
memory:
docker:
compose:
go:
```

## Notes

- `scripts/verify-cold-start.sh` clears the stack and its volumes first, so the
  measurement includes image pulls only if they are not already cached. Record
  whether the image cache was warm, since it dominates the result.
- SC-009's clock starts when the workflow is queued, not when a job begins.
