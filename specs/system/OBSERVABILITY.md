# OBSERVABILITY — Notification System

Both services are instrumented with the OpenTelemetry Go SDK. Metrics are scraped by Prometheus
and visualised in Grafana. Traces are exported via OTLP to Grafana Tempo.

---

## OpenTelemetry Instrumentation

| Concern | Tool |
|---------|------|
| SDK | `go.opentelemetry.io/otel` |
| Metrics exporter | Prometheus (`go.opentelemetry.io/otel/exporters/prometheus`) |
| Trace exporter | OTLP gRPC → OTel Collector → Grafana Tempo |
| Logging | `log/slog` (OTel logs API not yet stable in Go) |

### Metrics

| Metric | Type | Labels | Recorded by |
|--------|------|--------|-------------|
| `notification.sent` | Counter | `channel` | Processor |
| `notification.failed` | Counter | `channel` | Processor |
| `notification.attempts` | Counter | `channel` | Processor |
| `notification.delivery.latency_ms` | Histogram | `channel` | Processor |
| `notification.queue.depth` | Gauge | `priority` | API (reads `XLEN` on scrape) |
| `ratelimiter.tokens` | Gauge | `channel` | Processor |

### Traces

| Span | Service | Notes |
|------|---------|-------|
| HTTP request | API | root span per request |
| DB query | API | child of HTTP request span |
| Stream publish | API | child of HTTP request span |
| Stream message processing | Processor | root span per message |
| Delivery HTTP call | Processor | child of message span |

---

## Structured Logging

**Format:** JSON, one object per line.

Required fields on every log line:

| Field | Value |
|-------|-------|
| `ts` | ISO8601 timestamp |
| `level` | `debug \| info \| warn \| error` |
| `msg` | human-readable description |
| `service` | `notification-api \| notification-processor` |
| `version` | `1.0.0` |

Log level configurable via `LOG_LEVEL` env var (default: `info`).

### Correlation ID

Every HTTP request gets a `X-Correlation-ID` header (generated if absent). It propagates through:
- HTTP response headers
- All log lines for that request lifecycle
- Outbound webhook calls (`X-Correlation-ID` header)

---

## Infrastructure (docker-compose.yml)

| Service | Purpose | Default URL |
|---------|---------|-------------|
| `otel-collector` | Receives OTLP spans; forwards traces to Tempo, metrics to Prometheus | — |
| `prometheus` | Scrapes `/metrics` on both services | `http://localhost:9090` |
| `tempo` | Stores traces; queried by Grafana | `http://localhost:3200` |
| `grafana` | Dashboards (metrics via Prometheus, traces via Tempo, logs via Loki) | `http://localhost:3000` |

---

## Health Check

`GET /health` on the Notification Management API only.

| Check | Implementation | Failure condition |
|-------|---------------|------------------|
| PostgreSQL | `SELECT 1`, 2s timeout | Error or timeout |
| Redis | `PING`, 1s timeout | Error or timeout |

Returns `200 OK` if all checks pass, `503 Service Unavailable` if any fail.

The Notification Processor has no HTTP endpoint; its liveness is observable via metrics and logs.

---

## Key Log Events

```
notification.created          info   {id, channel, priority}
notification.enqueued         info   {id, priority, stream}
notification.processing       info   {id, attempt, channel}
notification.delivered        info   {id, channel, latency_ms}
notification.retry_scheduled  warn   {id, attempt, next_attempt_at, delay_ms}
notification.failed           error  {id, channel, attempts, last_error}
notification.cancelled        info   {id}
notification.duplicate        warn   {idempotency_key, existing_id, key_type}
ratelimit.throttled           warn   {channel}
worker.lock_missed            debug  {id}
```
