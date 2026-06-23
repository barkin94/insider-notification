# Delivery Scheduler Service

Manages scheduling of notifications for future delivery. Consumes `NotificationsScheduledEvent` messages from scheduled notifications created via the API, persists them to Postgres with a `scheduled_at` timestamp, and republishes them to the appropriate priority delivery topic once the scheduled time has passed.

Scales horizontally — the dispatcher uses `SELECT ... FOR UPDATE SKIP LOCKED` so multiple instances claim disjoint rows with no duplicate processing.

---

## Directory Structure

```text
deliveryscheduler/
├── cmd/
│   └── main.go                                          # Entry point
├── internal/
│   ├── app/
│   │   └── app.go                                       # Dependency wiring and lifecycle
│   ├── config/
│   │   └── config.go                                    # Environment variable loading
│   ├── db/
│   │   ├── scheduled_notification.go                    # ScheduledNotification entity
│   │   ├── scheduled_notification_repo.go               # Repository interface
│   │   └── postgres/
│   │       ├── repository.go                            # PostgreSQL implementation (Bun ORM)
│   │       └── repository_test.go                       # Repository tests
│   ├── scheduled_notification_dispatcher/
│   │   └── dispatcher.go                                # Scheduler dispatcher logic
│   └── transport/
│       └── messaging/
│           ├── notifications_scheduled_consumer.go      # Consumes NotificationsScheduledEvent
│           └── notification_schedule_cancelled_consumer.go # Consumes NotificationScheduleCancelledEvent
└── migrations/                                          # SQL migration files
```

---

## Architecture

```
NotificationsScheduledEvent (from API)
    │
    ▼
Consumer
    │  persists event as ScheduledNotification row
    ▼
PostgreSQL (scheduled_notifications table)
    │
    ├─ Dispatcher (ticker, default: every 1s)
    │     polls WHERE scheduled_at <= NOW()
    ▼
NATS JetStream (`notify.scheduled.due`)
    │
    ▼
API Service (ScheduledDueConsumer — hydrates and publishes to priority topics)
```

### Components

| Component | File | Responsibility |
| --- | --- | --- |
| Consumer | `internal/transport/messaging/notifications_scheduled_consumer.go` | Subscribes to `TopicNotificationScheduled`, batch-upserts events to Postgres with `scheduled_at = ScheduledAt` |
| CancelConsumer | `internal/transport/messaging/notification_schedule_cancelled_consumer.go` | Subscribes to `TopicNotificationScheduleCancelled`, deletes the matching row |
| Dispatcher | `internal/scheduled_notification_dispatcher/dispatcher.go` | Ticks every interval, atomically claims and deletes due rows, publishes as `ScheduledNotificationDueEvent` |
| Repository | `internal/db/scheduled_notification_repo.go` | Repository interface for `scheduled_notifications` |

---

## Persistence

**Database:** PostgreSQL via [Bun ORM](https://bun.uptrace.dev/)

**Table:** `scheduled_notifications`

| Column | Type | Notes |
| --- | --- | --- |
| `notification_id` | UUID | Primary key (one scheduled delivery per notification) |
| `scheduled_at` | TIMESTAMPTZ | When to republish; set from `ScheduledAt` in the event |

---

## Atomic Claim & Delete

The dispatcher uses **`SELECT ... FOR UPDATE SKIP LOCKED`** to prevent duplicate processing across concurrent instances:

1. Locks rows where `scheduled_at <= NOW()` (exclusive, non-blocking)
2. Concurrent queries **skip locked rows** and return the next batch
3. Each instance claims a **disjoint set** without collision
4. Rows are deleted atomically with `RETURNING *` — no re-fetch needed

---

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `DATABASE_URL` | — | PostgreSQL DSN (required) |
| `NATS_ADDR` | — | NATS address, e.g. `nats://localhost:4222` (required) |
| `DELIVERY_SCHEDULER_BATCH_SIZE` | `100` | Max scheduled notifications claimed per tick |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `OTEL_ENABLED` | `false` | Enable OpenTelemetry export |
| `OTEL_SERVICE_NAME` | `deliveryscheduler` | Service name reported to the collector |
| `OTEL_ENDPOINT` | — | gRPC endpoint for the OTel collector |

Copy `.env.example` to `.env` and fill in the required values.

---

## Prerequisites

- Docker and Docker Compose
- [Go 1.25+](https://go.dev/dl/) — only needed when running directly

---

## Running the service

### 1. Configure environment

```bash
cp deliveryscheduler/.env.example deliveryscheduler/.env
```

### 2. Start the service

```bash
# With Docker Compose (recommended — runs migrations automatically via migrate-deliveryscheduler)
docker compose up deliveryscheduler

# Directly (requires PostgreSQL and NATS reachable on localhost)
go run ./cmd/main.go
```

When using Docker Compose, the `migrate-deliveryscheduler` container runs the SQL migrations before the service starts.
