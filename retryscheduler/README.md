# Retry Scheduler Service

Manages retry scheduling for the notification delivery system. Consumes `NotificationRetryScheduleEvent` messages from the retry topic, persists them to Postgres with a `retry_after` timestamp, and republishes them to the appropriate priority delivery topic once the scheduled time has passed.

Scales horizontally — the dispatcher uses `SELECT ... FOR UPDATE SKIP LOCKED` so multiple instances claim disjoint rows with no duplicate processing.

---

## Directory Structure

```text
retryscheduler/
├── cmd/
│   └── main.go                                          # Entry point
├── internal/
│   ├── app/
│   │   └── app.go                                       # Dependency wiring and lifecycle
│   ├── config/
│   │   └── config.go                                    # Environment variable loading
│   ├── db/
│   │   ├── model.go                                     # DeliveryAttempt entity
│   │   └── delivery_attempt_repo.go                     # PostgreSQL repository
│   └── transport/
│       └── messaging/
│           ├── retry_consumer.go                        # Consumes TopicRetry, persists to Postgres
│           ├── retrydispatcher.go                       # Polls Postgres, republishes due retries
│           └── topics.go                                # Priority → topic mapping
└── migrations/                                          # SQL migration files
```

---

## Architecture

```text
<retry_topic> (notify:stream:retry)
    │
    ▼
RetryConsumer
    │  persists NotificationRetryScheduleEvent as DeliveryAttempt row
    ▼
PostgreSQL (delivery_attempts table)
    ▲
    │  polls WHERE retry_after <= NOW()
RetryDispatcher (ticker, default: every 1s)
    │
    ▼
Redis Streams (high / normal / low priority topics)
    │
    ▼
Processor Service (re-delivers)
```

### Components

| Component | File | Responsibility |
| --- | --- | --- |
| RetryConsumer | `internal/transport/messaging/retry_consumer.go` | Subscribes to TopicRetry, upserts each event to Postgres with `retry_after = ScheduledAt` |
| RetryDispatcher | `internal/transport/messaging/retrydispatcher.go` | Ticks every interval, atomically claims and deletes due rows (`FOR UPDATE SKIP LOCKED`), publishes each as `NotificationReadyEvent` via concurrent goroutines; re-enqueues any publish failures in a single batch upsert |
| DeliveryAttemptRepository | `internal/db/delivery_attempt_repo.go` | PostgreSQL CRUD for `delivery_attempts` |

---

## Persistence

**Database:** PostgreSQL via [Bun ORM](https://bun.uptrace.dev/)

**Table:** `delivery_attempts`

| Column | Type | Notes |
| --- | --- | --- |
| `notification_id` | UUID | Primary key (one active attempt per notification) |
| `attempt_number` | INT | Current attempt count, carried from the processor event |
| `priority` | VARCHAR | `high`, `normal`, `low` |
| `channel` | VARCHAR | `sms`, `email`, `push` |
| `recipient` | VARCHAR | Delivery target |
| `content` | TEXT | Notification body |
| `max_attempts` | INT | Upper limit on retries |
| `retry_after` | TIMESTAMPTZ | When to republish; set from `ScheduledAt` in the retry event |

---

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `DATABASE_URL` | — | PostgreSQL DSN (required) |
| `REDIS_ADDR` | — | Redis address, e.g. `localhost:6379` (required) |
| `RETRY_DISPATCH_INTERVAL` | `1s` | How often the dispatcher polls for due retries |
| `RETRY_DISPATCH_BATCH_SIZE` | `100` | Max retries republished per tick |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `OTEL_ENABLED` | `false` | Enable OpenTelemetry export |
| `OTEL_SERVICE_NAME` | `retryscheduler` | Service name reported to the collector |
| `OTEL_ENDPOINT` | — | gRPC endpoint for the OTel collector |

Copy `.env.example` to `.env` and fill in the required values.

---

## Prerequisites

- Docker and Docker Compose — for the containerised setup
- [Go 1.25+](https://go.dev/dl/) — only needed when running directly

---

## Running the service

### 1. Configure environment

```bash
cp retryscheduler/.env.example retryscheduler/.env
```

### 2. Start the service

```bash
# With Docker Compose (recommended — runs migrations automatically via migrate-processor)
docker compose up retryscheduler

# Directly (requires PostgreSQL and Redis reachable on localhost)
go run ./cmd/main.go
```

When using Docker Compose, the `migrate-processor` container runs the SQL migrations before the service starts.
