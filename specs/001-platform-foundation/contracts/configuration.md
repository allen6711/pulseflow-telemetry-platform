# Contract: Configuration

**Feature**: `001-platform-foundation` | **Consumers**: every later feature

All configuration comes from environment variables. Every setting has a default that works for
local development, so a developer can start the stack with no variables set at all. Every setting is
validated at startup, and validation failure prevents the process from reaching a running state.

Prefix: `PULSEFLOW_`. Later features extend this table; they do not introduce a second mechanism.

## Variables

### Identity

| Variable | Type | Default | Valid values |
| --- | --- | --- | --- |
| `PULSEFLOW_SERVICE_NAME` | string | `pulseflow-api` (api) / `pulseflow-worker` (worker) | non-empty |
| `PULSEFLOW_VERSION` | string | `dev` | non-empty; set from git describe at build time |
| `PULSEFLOW_ENVIRONMENT` | string | `local` | `local`, `ci`, `benchmark` |

### HTTP server

| Variable | Type | Default | Valid values |
| --- | --- | --- | --- |
| `PULSEFLOW_HTTP_PORT` | int | `8080` (api) / `8081` (worker) | 1–65535 |
| `PULSEFLOW_SHUTDOWN_GRACE_PERIOD` | duration | `30s` | > 0, ≤ `5m` |

### Logging

| Variable | Type | Default | Valid values |
| --- | --- | --- | --- |
| `PULSEFLOW_LOG_LEVEL` | string | `info` | `debug`, `info`, `warn`, `error` |

### Kafka

| Variable | Type | Default | Valid values |
| --- | --- | --- | --- |
| `PULSEFLOW_KAFKA_BROKERS` | comma-separated list | `localhost:9092` | at least one `host:port` |

### ClickHouse

| Variable | Type | Default | Valid values |
| --- | --- | --- | --- |
| `PULSEFLOW_CLICKHOUSE_ADDR` | string | `localhost:9000` | `host:port` (native protocol) |
| `PULSEFLOW_CLICKHOUSE_DATABASE` | string | `pulseflow` | non-empty |
| `PULSEFLOW_CLICKHOUSE_USER` | string | `default` | non-empty |
| `PULSEFLOW_CLICKHOUSE_PASSWORD` | string · **sensitive** | `""` | any |

### Redis

| Variable | Type | Default | Valid values |
| --- | --- | --- | --- |
| `PULSEFLOW_REDIS_ADDR` | string | `localhost:6379` | `host:port` |
| `PULSEFLOW_REDIS_PASSWORD` | string · **sensitive** | `""` | any |

### Health checking

| Variable | Type | Default | Valid values |
| --- | --- | --- | --- |
| `PULSEFLOW_HEALTH_CHECK_TIMEOUT` | duration | `2s` | > 0, ≤ `30s` |
| `PULSEFLOW_HEALTH_CACHE_TTL` | duration | `1s` | ≥ 0, strictly less than `PULSEFLOW_HEALTH_CHECK_TIMEOUT` |

## Error contract

Validation collects every failure before reporting, rather than stopping at the first. A developer
with three mistakes in their environment sees three lines once, not one line three restarts in a
row.

Each line names the variable, quotes the value received, and states the permitted format or range:

```text
configuration invalid (3 errors):
  PULSEFLOW_HTTP_PORT: got "http", must be an integer between 1 and 65535
  PULSEFLOW_LOG_LEVEL: got "verbose", must be one of debug, info, warn, error
  PULSEFLOW_HEALTH_CACHE_TTL: got "5s", must be less than PULSEFLOW_HEALTH_CHECK_TIMEOUT (2s)
```

The process then exits with a non-zero status. It does not start an HTTP listener, does not connect
to any dependency, and does not enter a partially initialized state (FR-017).

## Sensitive values

Variables marked **sensitive** are masked as `***` wherever configuration is rendered — the startup
summary log, any diagnostic output, and the readiness response. They are held in memory in cleartext
only for the client connections that need them.

## Rules for later features

1. New settings extend this table; no feature introduces a second configuration mechanism (config
   files, flags, remote config).
2. Every new setting has a default usable for local development. A setting with no sensible default
   means the feature is under-specified.
3. Every new setting is validated at startup with an error message matching the format above.
4. A setting holding a credential or token is marked sensitive.
5. Changing an existing variable's name or default is a breaking change to this contract and
   requires updating this file in the same change.
