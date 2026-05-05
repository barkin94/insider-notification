# OBSERVABILITY — Notification System

## Structured Logging

**Library:** `go.uber.org/zap` (production logger)

**Format:** JSON, one object per line

**Required fields on every log line:**
```json
{
  "ts":             "2024-06-01T09:00:00.000Z",
  "level":          "info | warn | error",
  "msg":            "human readable description",
  "correlation_id": "uuid",        ← injected by middleware on every request
  "service":        "notification-api | notification-processor",
  "version":        "1.0.0"
}
```

**Additional fields by context:**

HTTP request logs:
```json
{
  "method":         "POST",
  "path":           "/api/v1/notifications",
  "status":         201,
  "latency_ms":     14,
  "request_id":     "uuid"
}
```

Worker logs:
```json
{
  "notification_id": "uuid",
  "channel":         "sms",
  "attempt":         2,
  "event":           "delivery_attempt | retry_scheduled | exhausted"
}
```

Delivery logs:
```json
{
  "notification_id":   "uuid",
  "channel":           "sms",
  "provider_latency_ms": 187,
  "http_status":       202,
  "provider_message_id": "uuid"
}
```

---

## Correlation ID Middleware

Every incoming HTTP request gets a `X-Correlation-ID` header injected (or passed through
if the client supplies one). The ID propagates through:

- HTTP response headers (`X-Correlation-ID`)
- All log lines for that request lifecycle
- All downstream calls (e.g. webhook.site request includes `X-Correlation-ID` header)

**Implementation:**
```go
func CorrelationIDMiddleware(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    id := r.Header.Get("X-Correlation-ID")
    if id == "" {
      id = uuid.New().String()
    }
    ctx := context.WithValue(r.Context(), correlationIDKey, id)
    w.Header().Set("X-Correlation-ID", id)
    next.ServeHTTP(w, r.WithContext(ctx))
  })
}
```

---

## Metrics

Metrics are stored in-memory (atomic counters + ring buffers for latency) and exposed
via `GET /metrics`. No external metrics system (Prometheus, etc.) is required for this scope.

**Counters (atomic int64, reset on restart):**
```
sent_total_{channel}        ← incremented on each successful delivery
failed_total_{channel}      ← incremented on each exhausted notification
attempts_total_{channel}    ← incremented on each delivery attempt
```

**Latency tracking:**
- Ring buffer of last 1000 latency values per channel
- `avg_latency_ms` computed as mean of ring buffer on each `/metrics` request
- Latency = time from worker pickup to provider response

**Queue depth:**
- Read from Redis via `XLEN notify:stream:{priority}` on each `/metrics` request
- Also maintained as Redis counters for fast access (reconciled on startup)

**Rate limiter state:**
- Token count read from Redis on each `/metrics` request via Lua script (atomic read)

---

## Health Check

`GET /health` (Notification Management API) performs active checks:

| Check | Implementation | Failure condition |
|-------|---------------|------------------|
| PostgreSQL | `SELECT 1` with 2s timeout | Error or timeout |
| Redis | `PING` with 1s timeout | Error or timeout |

Returns `200 OK` if all checks pass, `503 Service Unavailable` if any fail.

The Notification Processor does not expose an HTTP health endpoint; its liveness is observed via metrics counters and structured logs.

---

## Log Levels

| Level | When to use |
|-------|------------|
| `debug` | Worker poll cycles, lock acquisition (disabled in production) |
| `info` | Request received/completed, notification created, delivered |
| `warn` | Retry scheduled, rate limiter token exhausted, idempotency hit |
| `error` | Delivery failure, DB error, Redis error, provider non-retryable error |

Log level configurable via `LOG_LEVEL` environment variable (default: `info`).

---

## Key Log Events

```
notification.created          info   {id, channel, priority, scheduled_at}
notification.enqueued         info   {id, priority, queue}
notification.processing       info   {id, attempt, channel}
notification.delivered        info   {id, channel, provider_message_id, latency_ms}
notification.retry_scheduled  warn   {id, attempt, next_attempt_at, delay_ms}
notification.failed           error  {id, channel, attempts, last_error}
notification.cancelled        info   {id}
notification.duplicate        warn   {idempotency_key, existing_id, key_type}
ratelimit.throttled           warn   {channel, available_tokens}
worker.lock_missed            debug  {id}
worker.reconciliation         info   {requeued_count}
```
