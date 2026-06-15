# Processor Service

Consumes notification events from prioritised message topics and delivers them via a weighted priority router, a four-gate delivery pipeline, and an exponential-backoff retry mechanism.

---

## Directory Structure

```text
processor/
├── cmd/
│   └── main.go                                          # Entry point
├── internal/
│   ├── app/
│   │   └── app.go                                       # Dependency wiring and lifecycle
│   ├── config/
│   │   └── config.go                                    # Environment variable loading
│   ├── delivery/
│   │   ├── priorityrouter.go                            # Weighted round-robin scheduler
│   │   ├── pipeline.go                                  # Four-gate delivery pipeline
│   │   └── workerpool.go                               # Concurrent worker pool
│   ├── service/
│   │   ├── ntfndeliveryclient.go                        # HTTP client for the delivery provider
│   │   ├── ratelimit.go                                 # Per-channel token-bucket rate limiter
│   │   ├── retry.go                                     # Retry backoff formula
│   │   └── metrics.go                                   # OpenTelemetry metrics
│   └── transport/
│       └── messaging/
│           └── router.go                                # Redis Stream consumer setup
```

---

## Architecture Overview

```text
Redis Streams (high / normal / low)
    │
    ▼
PriorityRouter (weighted round-robin)
    │
    ▼
WorkerPool (N concurrent workers)
    │
    ▼
Delivery Pipeline (lock → rate-limit → send → record)
    ├──► Notification Provider (HTTP POST)
    └──► TopicRetry (NotificationRetryScheduleEvent — handled by retryscheduler)
```

Retry scheduling is handled by the [`retryscheduler`](../retryscheduler/README.md) service. The processor is stateless — it only reads from and writes to Redis.

### Layers

| Layer | Package | Responsibility |
| --- | --- | --- |
| Delivery | `internal/delivery` | Priority routing, worker pool, four-gate pipeline |
| Service | `internal/service` | HTTP delivery client, rate limiting, retry backoff |
| Messaging | `internal/transport/messaging` | Redis Stream consumer setup |

---

## PriorityRouter — `internal/delivery/priorityrouter.go`

Schedules work across multiple channels using **weighted round-robin**. Higher-weight sources get proportionally more slots in the rotation schedule.

### Construction

Given weights `high=3, normal=2, low=1`, the router pre-expands them into a slots array:

```text
sources: [ high_ch   normal_ch   low_ch ]
           idx 0     idx 1       idx 2

slots:   [ 0, 0, 0,  1, 1,  2 ]
           H  H  H   N  N   L
```

Each `Next()` call picks the next slot via `cursor % len(slots)` and advances the cursor.

### Decision tree per `Next()` call

```text
Next() called
│
├─ Phase 1: scheduled channel has a message? (non-blocking)
│   yes → return it                     weights respected, fast path
│   no  ↓
│
├─ Phase 2: any channel has a message? (non-blocking scan, high → normal → low)
│   yes → return highest-priority one available
│   no  ↓
│
└─ Phase 3: block on high_ch with 1s idle timeout
    woke by message → return (value, true)
    timeout (1s)    → return (zero, false)  → WorkerPool retries from Phase 1
    ctx cancelled   → return (zero, false)  → WorkerPool exits
```

**Phase 1** enforces weighting — it only fires when the rotation lands on that source's slot.

**Phase 2** prevents wasted turns — if the scheduled channel is empty but another has work, it is taken regardless of whose slot it was.

**Phase 3** avoids busy-waiting when all channels are idle. Parking on `sources[0]` biases wake-ups toward high-priority messages. Normal/low messages that arrive while parked are picked up within 1s when the timeout fires and Phase 2 runs again.

---

## Delivery Pipeline — `internal/delivery/pipeline.go`

Each message pulled from the router runs through four gates in sequence:

```text
NotificationReadyEvent
│
├─ 1. Lock          TryLock(notificationID)
│                   miss (already being processed) → Ack, skip
│                   error                          → Nack
│
├─ 2. Rate limit    IsAllowed(channel)
│                   allowed  → continue
│                   limited  → publish NotificationRetryScheduleEvent to TopicRetry, Ack
│                   error    → Nack
│
├─ 3. Send          HTTP POST to notification provider
│                   202 Accepted        → success
│                   400 / 401 / 403     → non-retryable failure
│                   anything else / err → retryable failure
│
└─ 4. Record outcome
        success           → publish delivered status
        retryable failure → publish NotificationRetryScheduleEvent to TopicRetry
        terminal failure  → publish failed status
```

The pipeline Acks the message at the end of a successful gate sequence. It Nacks only when state could not be persisted (so the message is redelivered by the broker).

---

## Published Events

Terminal outcomes (success or exhausted retries) are published to the status topic as `NotificationDeliveryResultEvent`. Retry-eligible outcomes are published to the retry topic as `NotificationRetryScheduleEvent` and consumed by the [`retryscheduler`](../retryscheduler/README.md) service.

| Field                 | Notes                              |
|-----------------------|------------------------------------|
| `notification_id`     |                                    |
| `status`              | `delivered` or `failed`            |
| `attempt_number`      | which attempt produced this result |
| `http_status_code`    |                                    |
| `provider_message_id` | set on success                     |
| `error_message`       | set on failure                     |
| `latency_ms`          |                                    |

---

## Retry Mechanism

When a delivery attempt is rate-limited or fails with a retryable error, the processor publishes a `NotificationRetryScheduleEvent` to `TopicRetry` with a `ScheduledAt` timestamp computed from the backoff formula:

```text
delay = min(60s × 2^(attempt−1), 480s) + uniform jitter in [0, delay × 0.2]
```

| Attempt       | Base delay | Max with jitter |
|---------------|------------|-----------------|
| 2 (1st retry) | 60s        | 72s             |
| 3             | 120s       | 144s            |
| 4             | 240s       | 288s            |

Default max attempts is **4** (3 retries). A per-notification `max_attempts` override is accepted at creation time.

The [`retryscheduler`](../retryscheduler/README.md) service consumes `TopicRetry`, persists the scheduled attempt to Postgres, and republishes it to the appropriate priority topic once `ScheduledAt` is past.

---

## Observability

- **Structured logging** on every delivery attempt and background event via the shared logger.
- **OpenTelemetry** (optional): traces, metrics, and logs exported to a gRPC endpoint. Trace context is propagated through Redis message metadata so spans stitch across service boundaries.

---

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `REDIS_ADDR` | — | Redis address, e.g. `localhost:6379` (required) |
| `WORKER_CONCURRENCY` | `10` | Number of concurrent delivery workers |
| `NTFN_DELIVERY_CLIENT_URL` | `http://localhost:8080` | Notification provider base URL |
| `NTFN_DELIVERY_CLIENT_TIMEOUT` | `10s` | HTTP client timeout |
| `HIGH_WEIGHT` | `3` | Router weight for the high-priority stream |
| `NORMAL_WEIGHT` | `2` | Router weight for the normal-priority stream |
| `LOW_WEIGHT` | `1` | Router weight for the low-priority stream |
| `SMS_RATE_PER_SECOND` | `10` | Token-bucket fill rate for SMS |
| `SMS_BURST` | `15` | Token-bucket burst capacity for SMS |
| `EMAIL_RATE_PER_SECOND` | `100` | Token-bucket fill rate for email |
| `EMAIL_BURST` | `120` | Token-bucket burst capacity for email |
| `PUSH_RATE_PER_SECOND` | `500` | Token-bucket fill rate for push |
| `PUSH_BURST` | `600` | Token-bucket burst capacity for push |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `OTEL_ENABLED` | `false` | Enable OpenTelemetry export |
| `OTEL_SERVICE_NAME` | — | Service name reported to the collector |
| `OTEL_ENDPOINT` | — | gRPC endpoint for the OTel collector |

Copy `.env.example` to `.env` and fill in the required values.

---

## Prerequisites

- [Go 1.23+](https://go.dev/dl/) — only needed when running directly
- Docker and Docker Compose — for the containerised setup

Run `make help` from the repo root to see all available commands.

---

## Running the system

### 1. Configure environment

Copy `.env.example` to `.env` inside the `processor/` folder. The example file is pre-filled for Docker Compose.

```bash
cp processor/.env.example processor/.env
```

### 2. Start the service

```bash
# With Docker Compose (recommended)
docker compose up processor

# Directly (requires Redis and mock-ntfn-provider reachable on localhost)
go run ./cmd/main.go
```

The processor has no database dependency. Retry scheduling is handled by the `retryscheduler` service.

### 3. Verify

The processor exposes metrics at `http://localhost:8081/metrics` and can be observed via Grafana at `http://localhost:3000`.
