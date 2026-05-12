# OBSERVABILITY — Processor Service

Metrics are exported via the OTel SDK → OTel Collector → Prometheus. Traces are exported via
OTLP gRPC to Tempo. Both are visualised in Grafana.

---

## Metrics

The Grafana dashboard (`grafana/provisioning/dashboards/notification.json`) expects these
metric names. The processor must emit all four.

### `notification_queue_depth` — Gauge

Current number of messages waiting in each priority Redis stream.

| Label | Values |
|-------|--------|
| `priority` | `high \| normal \| low` |

Updated continuously by the priority router. Drives the **Queue Depth by Priority** panel.

---

### `notification_sent_total` — Counter

Incremented once per successfully delivered notification (webhook returned 2xx).

No labels required by the dashboard. Drives `rate(notification_sent_total[1m])` in the
**Delivery Rate** panel.

---

### `notification_failed_total` — Counter

Incremented once when a notification exhausts all attempts and is marked `failed`.
Not incremented for retryable failures that will be re-queued.

No labels required by the dashboard. Drives `rate(notification_failed_total[1m])` in
**Delivery Rate** and `increase(notification_failed_total[1h])` in **Failed Deliveries (1h)**.

---

### `notification_delivery_latency_ms` — Histogram

Round-trip time of the outbound webhook HTTP call, in milliseconds.

No labels required by the dashboard. Drives the p50/p95/p99 quantiles in the
**Delivery Latency** panel.

Suggested bucket boundaries (ms): `50, 100, 250, 500, 1000, 2500, 5000, 10000`.

---

## Traces

The `processOne` function starts an OTel span named `processOne` for every message consumed.
Child spans should cover the discrete steps with meaningful names:

| Span | Covers |
|------|--------|
| `cancellation_check` | Redis key lookup |
| `acquire_lock` | Redis distributed lock |
| `rate_limit_check` | Redis rate limiter |
| `webhook_send` | Outbound HTTP call to delivery target |

Traces are exported via OTLP gRPC to Tempo and queryable in Grafana.

---

## Log fields

All log lines use structured `slog` with at minimum:

| Field | Type | Notes |
|-------|------|-------|
| `notification_id` | string (UUID) | present on every worker log line |
| `error` | string | present on error paths |
| `channel` | string | `sms \| email \| push` |
| `attempt` | int | current attempt number |

Logs are shipped to Loki via the OTel Collector and queryable in Grafana.
