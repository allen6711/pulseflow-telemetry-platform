# PulseFlow — Distributed Telemetry Analytics Platform

PulseFlow is a distributed telemetry ingestion and analytics platform for processing high-volume service events, storing time-series aggregates, serving low-latency analytics queries, and exposing its own reliability signals.

The project is intentionally scoped as a production-oriented backend/data-platform exercise rather than a full observability product. Its focus is distributed ingestion, fault-tolerant processing, analytical storage, caching, measurable performance, and operational visibility.

> Status: planned / under development. Benchmark values in this README must be replaced with measured results before they are presented as project outcomes.

## Why this project

PulseFlow is designed to demonstrate practical software-engineering skills that are difficult to show with a CRUD application:

- distributed event processing and horizontal scaling;
- partitioning, consumer groups, retries, idempotency, and dead-letter handling;
- analytical storage and query design;
- caching and p95 latency optimization;
- Docker/Kubernetes deployment;
- OpenTelemetry/Prometheus instrumentation;
- repeatable load testing and failure testing.

## Architecture

```text
Synthetic Services / Load Generator
              |
              v
        Go Ingestion API
              |
              v
             Kafka
       telemetry.events
              |
      +-------+-------+
      |       |       |
      v       v       v
   Worker  Worker  Worker
      |       |       |
      +-------+-------+
              |
      validate / dedupe /
      aggregate / persist
              |
              v
          ClickHouse
              |
              v
        Analytics API
              |
        +-----+-----+
        |           |
        v           v
      Redis       Client
    query cache

OpenTelemetry + Prometheus observe API latency, throughput,
consumer lag, processing errors, cache hit rate, and worker health.
```

## Planned technology stack

| Area | Technology |
| --- | --- |
| Language | Go |
| HTTP API | Go `net/http` or lightweight router |
| Event streaming | Kafka |
| Analytics store | ClickHouse |
| Cache | Redis |
| Observability | OpenTelemetry, Prometheus |
| Containers | Docker |
| Orchestration | Kubernetes |
| Local environment | Docker Compose |
| Load testing | k6 |
| CI | GitHub Actions |

## Core MVP

### 1. Telemetry ingestion

`POST /v1/events` accepts service telemetry events and publishes valid events to Kafka.

Example:

```json
{
  "event_id": "0198f0b4-2c0d-7a52-b4f3-a81319ab0001",
  "service": "payment-service",
  "timestamp": "2026-08-07T06:30:00Z",
  "metric": "api_latency_ms",
  "value": 184.2,
  "tags": {
    "environment": "benchmark",
    "region": "west-us"
  }
}
```

The API validates required fields and returns `202 Accepted` after Kafka acknowledges the publish operation.

### 2. Partitioned event processing

A worker service consumes `telemetry.events` with a Kafka consumer group. Multiple worker replicas must process partitions in parallel and support horizontal scaling.

Required processing behavior:

- validate event version and schema;
- reject malformed events;
- deduplicate by `event_id`;
- write analytical records to ClickHouse;
- commit Kafka offsets only after successful persistence;
- retry transient failures with bounded exponential backoff;
- route poison messages to a dead-letter topic after the retry limit.

### 3. Analytics queries

The analytics API exposes service-level aggregates for a requested time window.

Example:

```http
GET /v1/metrics/payment-service/api_latency_ms?from=...&to=...&percentiles=p50,p95,p99
```

The response should include count, average, min, max, and requested percentiles where applicable.

### 4. Redis query cache

Repeated analytics queries are cached using a deterministic cache key derived from service, metric, time range, and aggregation options.

The implementation must expose cache-hit and cache-miss metrics and use a short randomized TTL jitter so simultaneous keys do not all expire at the same instant.

### 5. Reliability and dead-letter handling

The system must demonstrate:

- worker restart and consumer-group rebalance;
- at-least-once delivery with application-level idempotency;
- no loss of Kafka-acknowledged events in the defined worker-restart test;
- inspectable dead-letter records containing the original payload and failure metadata.

### 6. Observability

Expose at least the following metrics:

- ingestion requests/sec;
- ingestion p50/p95/p99 latency;
- Kafka publish failures;
- consumer lag;
- processed events/sec;
- processing failures;
- dead-letter count;
- ClickHouse write latency;
- analytics-query p95 latency;
- Redis cache-hit ratio.

Traces should connect HTTP ingestion to Kafka publishing where practical and include trace IDs in structured logs.

## API summary

| Method | Endpoint | Purpose |
| --- | --- | --- |
| POST | `/v1/events` | Ingest one telemetry event |
| POST | `/v1/events/batch` | Ingest a bounded batch |
| GET | `/v1/metrics/{service}/{metric}` | Query aggregates |
| GET | `/v1/services` | List observed services |
| GET | `/v1/health/live` | Liveness probe |
| GET | `/v1/health/ready` | Dependency-aware readiness probe |
| GET | `/metrics` | Prometheus metrics |

## Benchmark plan

Benchmarks must be reproducible and run from committed scripts/configuration.

Minimum experiments:

1. **Throughput scaling** — run the same workload with 1, 2, and 4 workers.
2. **Query caching** — compare uncached and cached p95 analytics latency.
3. **Worker failure** — terminate a worker during ingestion and verify recovery.
4. **Duplicate delivery** — intentionally resend event IDs and verify exactly one analytical record per event ID.

Recommended development targets, not guaranteed outcomes:

- total generated events: at least 1,000,000;
- worker replicas: 1 / 2 / 4;
- target sustained throughput: roughly 10K–20K events/sec on a capable local machine;
- target p95 cached-query improvement: roughly 60–80%;
- worker-restart trials: 20;
- lost acknowledged events in defined restart trials: 0.

Actual hardware, command, dataset, configuration, run duration, and raw results must be documented.

## Testing

Required automated tests:

- event-schema validation;
- cache-key generation;
- idempotency behavior;
- retry classification;
- aggregation-query validation;
- API error mapping;
- ClickHouse repository integration test;
- Redis integration test;
- Kafka producer/consumer integration test.

End-to-end tests must exercise the local container stack.

## MVP acceptance criteria

The MVP is complete only when all of the following are true:

- `docker compose up` starts Kafka, ClickHouse, Redis, API, worker, and Prometheus dependencies needed for the demo;
- one million generated telemetry events can be submitted without manual intervention;
- at least three worker replicas can share processing through a consumer group;
- duplicate event IDs do not create duplicate analytical records;
- failed messages can reach a dead-letter topic after bounded retries;
- an analytics query returns correct aggregate values for a known fixture;
- Redis produces a measurable cached-query latency difference;
- Prometheus exposes throughput, lag, failure, and latency metrics;
- worker termination during the documented restart test does not lose Kafka-acknowledged events;
- benchmark scripts and raw/summary results are committed.

## Stretch goals

Only implement after MVP acceptance:

- gRPC ingestion endpoint;
- Kubernetes HPA based on consumer lag;
- Grafana dashboard;
- schema-version compatibility test;
- configurable retention/TTL policy;
- per-tenant rate limits.

## Explicit non-goals

- implementing Kafka, ClickHouse, or Redis from scratch;
- Raft/Paxos/consensus implementation;
- multi-region active-active deployment;
- service mesh;
- dozens of microservices;
- full commercial monitoring UI;
- AI/LLM features.

## Resume-ready measurements

Do not write target values as completed results. After benchmarking, the project should be able to support concise evidence such as:

> Built a distributed telemetry pipeline in Go with Kafka partitioned consumers, idempotent processing, retries, and dead-letter handling to process **[measured event count]** across **[measured worker count]** workers.

> Sustained **[measured throughput] events/sec** and reduced p95 analytics query latency by **[measured %]** using ClickHouse aggregation and Redis caching; recovered from **[N/N]** documented worker-restart trials with no lost Kafka-acknowledged events.

## Repository layout

```text
pulseflow/
├── cmd/
│   ├── api/
│   └── worker/
├── internal/
│   ├── api/
│   ├── config/
│   ├── telemetry/
│   ├── kafka/
│   ├── storage/
│   ├── cache/
│   └── observability/
├── migrations/
├── deployments/
│   ├── docker/
│   └── k8s/
├── tests/
│   ├── integration/
│   └── e2e/
├── benchmarks/
├── scripts/
├── docker-compose.yml
└── README.md
```

The exact package split may evolve, but the project should remain small enough that architecture can be understood quickly by a reviewer.
