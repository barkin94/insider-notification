# ARCHITECTURE — Notification System

## Component Overview

```
┌───────────────────────────────────────────────────────────┐
│               Notification Management API                  │
│   POST /notifications      GET /notifications/:id          │
│   POST /notifications/batch  POST /notifications/:id/cancel│
│   GET /notifications       GET /health                     │
└────────────┬──────────────────────────────────┬───────────┘
             │ writes                            │ reads
             ▼                                  │
    ┌──────────────────┐                        │
    │  PostgreSQL 16    │◄───────────────────────┘
    │  notifications    │
    │  delivery_attempts│  ◄── written by status event consumer
    │  idempotency_keys │
    └──────────────────┘
             │
             │ publishes to notify:stream:{priority}
             ▼
┌───────────────────────────────────────────────────────────┐
│                       Redis 7                              │
│  notify:stream:high      notify:stream:normal              │
│  notify:stream:low       notify:stream:status              │
│  ratelimit:{channel}     notify:lock:{id}                  │
│  idempotency:{key}                                         │
└───────────────────────────────────────────────────────────┘
             │ consumes from notify:stream:{priority}
             ▼
┌───────────────────────────────────────────────────────────┐
│               Notification Processor                       │
│   Stream Consumer (10 workers)                             │
│         │                                                  │
│   Rate Limiter (token bucket, 100 msg/s per channel)       │
│         │                                                  │
│   Delivery Service (POST webhook.site/uuid)                │
│         │                                                  │
│   Retry Scheduler (exponential backoff + jitter, 4 max)    │
│         │                                                  │
│   Status Publisher (publishes to notify:stream:status)     │
└───────────────────────────────────────────────────────────┘
             │ consumes from notify:stream:status
             ▼
    ┌──────────────────────────────────┐
    │  API Status Event Consumer        │
    │  updates PostgreSQL on each event │
    └──────────────────────────────────┘
```

---

## Tech Stack

| Layer | Choice | Rationale |
|---|---|---|
| Language | Go 1.2x | Required by case study; excellent concurrency primitives |
| HTTP Framework | `net/http` + `chi` | Lightweight, idiomatic Go |
| Database | PostgreSQL 16 | ACID transactions, strong schema enforcement, UUID PKs, JSONB for metadata |
| Message Broker | Redis 7 (Streams) | `XADD`/`XREADGROUP`/`XACK`, consumer groups, built-in PEL for crash recovery |
| Rate Limiter | Redis token bucket | Distributed-safe, survives app restarts |
| Migrations | `golang-migrate` | SQL-file based, versioned up/down |
| PostgreSQL Driver | `pgx/v5` + `pgxpool` | High-performance Go PostgreSQL driver |
| Observability | OpenTelemetry Go SDK | Unified metrics + traces; industry standard |
| Metrics backend | Prometheus | Scrapes OTel Prometheus exporter on both services |
| Visualization | Grafana | Dashboards over Prometheus; trace UI via Jaeger |
| Logging | `slog` | Structured, high-performance |
| Config | `viper` | Env + file config, 12-factor compatible |
| Testing | `testify` + `go test` | Standard Go testing with assertions |
| API Docs | `swaggo/swag` | Generates OpenAPI from Go annotations |

---

## Code Architecture

Both services follow a **Handler / Service / Repository** pattern. Handlers are thin HTTP
adapters; business logic lives in the service layer; data access is isolated in repositories.
All cross-layer dependencies are expressed as interfaces and injected via constructors.
No globals, no `init()`.

**Notification Management API:**

| Layer | Package | Responsibility |
|-------|---------|----------------|
| HTTP handlers | `api/internal/handler/` | Decode request, validate input, call service, encode response |
| Business logic | `api/internal/service/` | Orchestrate repo + stream publisher; own domain rules |
| Data access | `api/internal/db/` | PostgreSQL via pgx; implements repository interfaces |
| Stream | `internal/shared/stream/` | Publish `NotificationCreatedEvent` to Redis Streams |

**Notification Processor:**

| Layer | Package | Responsibility |
|-------|---------|----------------|
| Stream consumer | `processor/internal/worker/` | Poll streams, dispatch to delivery service |
| Delivery | `processor/internal/delivery/` | HTTP POST to webhook provider |
| Rate limiting | `processor/internal/ratelimit/` | Redis token bucket per channel |
| Retry | `processor/internal/retry/` | Backoff formula; re-enqueue with `deliver_after` |
| Stream | `internal/shared/stream/` | Publish `NotificationDeliveryResultEvent` to status stream |

All dependencies are injected via constructors. No globals, no `init()`.

---

## Key Design Decisions

### ADR: Redis Streams as Priority Message Broker
- **Decision:** Three separate Redis Streams (`notify:stream:high`, `notify:stream:normal`, `notify:stream:low`) with consumer group `notify:cg:processor`. Workers use `XREADGROUP` — polling high first, falling back to normal, then low. A fourth stream `notify:stream:status` carries status events from Processor back to API.
- **Rationale:** Streams provide built-in consumer group semantics (at-least-once delivery), PEL for crash recovery via `XAUTOCLAIM`, and per-message acknowledgement. No separate broker dependency beyond Redis.
- **Tradeoff accepted:** Low-priority messages can starve under sustained high load. Acceptable for this scope.

### ADR: Redis Token Bucket for Rate Limiting
- **Decision:** Lua script executed atomically in Redis. Key: `ratelimit:{channel}`. Capacity: 100 tokens. Refill: 100/s. Burst: 120.
- **Rationale:** Distributed-safe across multiple Processor instances. Survives restarts.
- **Tradeoff accepted:** Adds Redis round-trip per dispatch. Negligible at this scale.

### ADR: Exponential Backoff with Jitter for Retries
- **Decision:** Failed deliveries re-enqueued into the same priority stream with `deliver_after` in the message payload.
- **Formula:** `delay = min(base * 2^attempt, max_delay) + jitter` where jitter ∈ `[0, delay * 0.2]`.
- **Tradeoff accepted:** Retry delays are approximate. Acceptable.

### ADR: Dual Idempotency Strategy
- **Decision:** Client-supplied `Idempotency-Key` header checked first (Redis, 24h TTL). If absent, `sha256(channel + recipient + content)` checked against `idempotency_keys` table (1h window).
- **Rationale:** Explicit consumer control + protection against accidental duplicates.
- **Tradeoff accepted:** Hash collisions theoretically possible but negligible.

### ADR: Two-Service Architecture
- **Decision:** Single monorepo, two entrypoints. API owns PostgreSQL and REST surface; Processor owns delivery. Shared `internal/shared/` packages for common types.
- **Rationale:** Independent scaling of ingestion vs. delivery.
- **Tradeoff accepted:** Inter-service communication via Redis round-trips. Negligible at this scale.

### ADR: Event-Driven Status Updates (Processor → API)
- **Decision:** Processor publishes delivery outcomes to `notify:stream:status`. API status consumer writes `delivery_attempts` rows and updates `notifications.status` in PostgreSQL.
- **Rationale:** Processor does not need its own database.
- **Tradeoff accepted:** Status updates are eventually consistent. Acceptable for this scope.

### ADR: Hexagonal Architecture (Ports and Adapters)
- **Decision:** Both services follow hexagonal architecture. Domain logic has no import dependency on infrastructure packages. All external systems (PostgreSQL, Redis, webhook.site) are accessed through interfaces defined near the domain and implemented in adapter packages.
- **Rationale:** Makes each adapter independently testable via mocks. Allows swapping infrastructure (e.g., delivery target, broker) without touching application logic. Natural fit for Go where interfaces are implicit and lightweight.
- **Tradeoff accepted:** Slightly more files than a flat structure. Justified by testability.

### ADR: OpenTelemetry for Observability
- **Decision:** Both services instrument with the OTel Go SDK. Metrics exported via Prometheus exporter; traces via OTLP → OTel Collector → Jaeger. No custom metrics store.
- **Rationale:** Industry standard; eliminates custom counter/ring buffer code; gives traces, metrics, and dashboards with no additional instrumentation effort.
- **Tradeoff accepted:** Adds four services to `docker-compose.yml` (otel-collector, prometheus, grafana, jaeger). Acceptable for this scope.

---

## Development Commands

| Purpose | Command |
|---------|---------|
| Build all | `go build ./...` |
| Vet | `go vet ./...` |
| Test all | `go test ./...` |
| Test with race detector | `go test -race ./...` |
| Lint | `golangci-lint run` |
| Generate API docs | `swag init -dir api` |
| Run migrations | `go run ./api migrate up` |
| Run API service | `go run ./api` |
| Run Processor | `go run ./processor` |
| Start full stack | `docker-compose up` |
