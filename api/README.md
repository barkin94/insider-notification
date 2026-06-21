# API Service

The API service is the entry point for the notification platform. It accepts notification requests over HTTP, persists them to PostgreSQL, and publishes ready notifications to Redis Streams for downstream processing by the Processor service.

---

## Directory Structure

```
api/
├── cmd/
│   └── main.go                             # Entry point
├── internal/
│   ├── app/
│   │   └── app.go                          # Dependency wiring and lifecycle
│   ├── config/
│   │   └── config.go                       # Environment variable loading
│   ├── domain/
│   │   └── notification/
│   │       ├── models.go                   # Domain model with validation
│   │       └── errors.go                   # Domain error types
│   ├── repository/
│   │   ├── notification.go                 # Repository interface
│   │   ├── notification_model.go           # DB entity model
│   │   ├── errors.go
│   │   └── postgres/
│   │       └── notification_repo.go        # PostgreSQL implementation (Bun ORM)
│   ├── scheduler/
│   │   └── scheduler.go                    # Background poller for scheduled notifications
│   ├── service/
│   │   └── notification.go                 # Business logic layer
│   └── transport/
│       ├── http/
│       │   ├── router.go                   # Chi router setup
│       │   ├── notification.go             # HTTP handlers
│       │   └── dtos.go                     # Request / response types
│       └── messaging/
│           ├── delivery_result_consumer.go # Consumes delivery status events from Redis
│           └── scheduled_due_consumer.go   # Consumes scheduled-due events from delivery scheduler
├── migrations/                             # SQL migration files
└── docs/                                   # Auto-generated Swagger docs
```

---

## Architecture Overview

```
HTTP Client
    │
    ▼
HTTP Handlers (Chi)
    │
    ▼
Service Layer
    ├──► PostgreSQL (persist notification)
    └──► Redis Streams (publish ready event)
             │
             ▼
         Processor Service

Background Goroutines
    ├── Scheduler              — polls DB every 5s for scheduled notifications, publishes to Redis
    ├── DeliveryConsumer       — reads status events from Redis, updates DB status
    └── ScheduledDueConsumer   — reads scheduled-due events from Delivery Scheduler, hydrates notifications, publishes to Redis
```

### Layers

| Layer | Package | Responsibility |
|---|---|---|
| Domain | `internal/domain/notification` | Models, validation, error codes |
| Repository | `internal/repository` | Persistence interface and PostgreSQL adapter |
| Service | `internal/service` | Business rules, routing notifications to priority topics |
| HTTP Transport | `internal/transport/http` | Request parsing, response serialization, error mapping |
| Messaging Transport | `internal/transport/messaging` | Redis Stream consumer for delivery results |
| Scheduler | `internal/scheduler` | Background ticker that dispatches delayed notifications |

---

## HTTP API

Base path: `/api/v1`

| Method | Path | Description |
|---|---|---|
| `POST` | `/notifications` | Create a single notification |
| `GET` | `/notifications` | List notifications with filtering and pagination |
| `GET` | `/notifications/{id}` | Get a notification by ID |
| `POST` | `/notifications/{id}/cancel` | Cancel a pending notification |
| `POST` | `/notifications/batch` | Create up to 1 000 notifications, returns 207 Multi-Status |

**Health endpoints:**

| Path | Description |
|---|---|
| `/api/v1/liveness` | Always 200 — process is alive |
| `/api/v1/readiness` | 200 when PostgreSQL and Redis are reachable, 503 otherwise |

Swagger UI is served at `/api/v1/docs/`.

### Request Lifecycle

1. Chi router matches the route and calls the handler.
2. The handler decodes and structurally validates the JSON body.
3. The domain model's setter methods enforce business rules (valid channel, recipient format, priority, etc.).
4. The service layer persists the notification and, if delivery is immediate, publishes a `NotificationReadyEvent` to the matching priority Redis Stream.
5. All handler functions are wrapped by a central `errHandler` that translates domain errors → 422, parsing errors → 422, and unknown errors → 500.

### Pagination

`GET /notifications` supports two pagination modes:

- **Keyset (default)**: Pass `cursor=<uuid>` and `limit`. Efficient for large tables.
- **Offset**: Pass `page` and `limit`. Falls back automatically when no cursor is provided.

Filters: `status`, `channel`, `batch_id`, `from` / `to` (created_at range).

---

## Notification Lifecycle

```
Created ──► Pending
                │
     ┌──────────┴──────────┐
     │                     │
  Delivered            Failed / Cancelled
```

- `pending` — stored, not yet dispatched or awaiting `deliver_after`
- `delivered` — processor confirmed successful delivery
- `failed` — processor exhausted max attempts
- `cancelled` — cancelled via API before delivery

Status transitions are atomic and use optimistic locking (checking the current status before updating) to prevent race conditions between the scheduler, the consumer, and concurrent API calls.

---

## Priority Routing

Notifications are routed to one of three Redis Streams based on their `priority` field:

| Priority | Stream |
|---|---|
| `high` | `notify:stream:high` |
| `normal` | `notify:stream:normal` |
| `low` | `notify:stream:low` |

This allows the Processor service to apply differentiated scheduling per priority lane.

---

## Scheduled Notifications

When a notification has a `deliver_after` timestamp set, it is **not** published to Redis at creation time. Instead, the background **Scheduler** goroutine polls the database every `SCHEDULER_INTERVAL` (default: 5s) for notifications where:

```sql
deliver_after IS NOT NULL AND deliver_after <= NOW() AND status = 'pending'
```

Up to 500 notifications are fetched per tick and published to the appropriate priority stream.

---

## Delivery Result Consumer

The `DeliveryResultConsumer` goroutine subscribes to the `notify:stream:status` Redis Stream (consumer group `notify:cg:api`). When the Processor service finishes a delivery attempt, it publishes a `NotificationDeliveryResultEvent` containing:

- `NotificationID`
- Final `Status` (`delivered` or `failed`)
- `AttemptNumber`, `HTTPStatusCode`, `ErrorMessage`, `LatencyMS`

The consumer updates the notification's status in PostgreSQL and acknowledges the message (Ack/Nack with 5s re-send sleep for at-least-once delivery).

---

## Persistence

**Database:** PostgreSQL via [Bun ORM](https://bun.uptrace.dev/)

**Table:** `notifications`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | Primary key |
| `batch_id` | UUID | Nullable, groups batch-created notifications |
| `recipient` | VARCHAR(255) | Email address, phone number, or device token |
| `channel` | VARCHAR(20) | `sms`, `email`, `push` |
| `content` | TEXT | Notification body |
| `priority` | VARCHAR(20) | `high`, `normal`, `low` |
| `status` | VARCHAR(20) | `pending`, `delivered`, `failed`, `cancelled` |
| `deliver_after` | TIMESTAMPTZ | Nullable — schedule future delivery |
| `max_attempts` | INT | Default 4 |
| `created_at` | TIMESTAMPTZ | |
| `updated_at` | TIMESTAMPTZ | |

**Indexes** cover `batch_id`, `status`, `channel`, `created_at DESC`, `(deliver_after, status)`, and `(status, updated_at)`.

---

## Observability

- **Structured logging** on every request and background event via the shared logger middleware.
- **OpenTelemetry** (optional): traces, metrics, and logs exported to a gRPC endpoint. Trace context is propagated through Redis message metadata so spans stitch across service boundaries.
- **Health checks** at `/liveness` and `/readiness` for use with container orchestrators.

---

## Configuration

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | PostgreSQL DSN (required) |
| `REDIS_ADDR` | — | Redis address, e.g. `localhost:6379` (required) |
| `PORT` | `8080` | HTTP server port |
| `SCHEDULER_INTERVAL` | `5s` | How often the scheduler polls for delayed notifications |
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

Copy `.env.example` to `.env` inside the `api/` folder. The example file is pre-filled for Docker Compose.

```bash
cp api/.env.example api/.env
```

### 2. Start the service

```bash
# With Docker Compose (recommended — runs migrations automatically)
docker compose up api

# Directly (requires PostgreSQL and Redis reachable on localhost)
go run ./cmd/main.go
```

When using Docker Compose, the `migrate-api` container runs the SQL migrations and exits before the service starts.

### 3. Verify

```bash
curl http://localhost:8080/api/v1/liveness
```

Swagger UI: `http://localhost:8080/api/v1/docs/index.html`
