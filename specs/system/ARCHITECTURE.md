# ARCHITECTURE — Notification System

## Component Overview

```
┌───────────────────────────────────────────────────────────┐
│               Notification Management API                  │
│   POST /notifications      GET /notifications/:id          │
│   POST /notifications/batch  POST /notifications/:id/cancel│
│   GET /notifications       GET /metrics    GET /health     │
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
             │ XADD notify:stream:{priority}
             ▼
┌───────────────────────────────────────────────────────────┐
│                       Redis 7                              │
│  notify:stream:high      notify:stream:normal              │
│  notify:stream:low       notify:stream:status              │
│  ratelimit:{channel}     notify:lock:{id}                  │
│  idempotency:{key}       metrics:*                         │
└───────────────────────────────────────────────────────────┘
             │ XREADGROUP notify:cg:processor
             ▼
┌───────────────────────────────────────────────────────────┐
│               Notification Processor                       │
│   Stream Consumer (10 workers)                             │
│         │                                                  │
│   Rate Limiter (Redis token bucket, 100 msg/s per channel) │
│         │                                                  │
│   Delivery Service (POST webhook.site/uuid)                │
│         │                                                  │
│   Retry Scheduler (exponential backoff + jitter, 4 max)    │
│         │                                                  │
│   Status Publisher (XADD notify:stream:status)             │
└───────────────────────────────────────────────────────────┘
             │ XREADGROUP notify:cg:api (status events)
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
| Message Broker | Redis 7 (Streams) | `XADD`/`XREADGROUP`/`XACK`, consumer groups, built-in pending entry list for crash recovery |
| Rate Limiter | Redis token bucket | Distributed-safe, survives app restarts |
| Migrations | `golang-migrate` | Most popular Go migration tool; SQL-file based, versioned up/down |
| PostgreSQL Driver | `pgx/v5` + `pgxpool` | Most popular high-performance Go PostgreSQL driver |
| Logging | `zap` | Structured, high-performance, supports correlation IDs |
| Config | `viper` | Env + file config, 12-factor compatible |
| Testing | `testify` + `go test` | Standard Go testing with assertions |
| API Docs | `swaggo/swag` | Generates OpenAPI from Go annotations |

---

## Project Layout

```
/
├── cmd/
│   ├── api/
│   │   └── main.go              # Notification Management API entrypoint
│   └── processor/
│       └── main.go              # Notification Processor entrypoint
├── internal/
│   ├── api/
│   │   ├── handler/             # HTTP handlers
│   │   └── middleware/          # logging, correlation ID
│   ├── config/                  # shared config loading (viper)
│   ├── db/                      # PostgreSQL connection (pgxpool)
│   │   └── migrations/          # golang-migrate SQL files (*.up.sql / *.down.sql)
│   ├── model/                   # shared domain structs
│   ├── stream/                  # Redis Streams producer (API) + consumer (Processor)
│   ├── ratelimit/               # token bucket Lua script implementation
│   ├── delivery/                # webhook.site HTTP client
│   ├── retry/                   # retry logic + backoff computation
│   ├── idempotency/             # key resolution + dedup logic
│   ├── metrics/                 # in-memory metrics store (atomic counters + ring buffer)
│   └── service/                 # orchestration layer
├── specs/                       # this directory
├── docker-compose.yml
├── Dockerfile.api
├── Dockerfile.processor
├── Makefile
└── docs/                        # swag-generated OpenAPI output
```

---

## Key Design Decisions

### ADR-1: Redis Streams as Priority Message Broker
- **Decision:** Three separate Redis Streams (`notify:stream:high`, `notify:stream:normal`, `notify:stream:low`) with consumer group `notify:cg:processor`. Workers use `XREADGROUP` — polling high first, falling back to normal, then low. A fourth stream `notify:stream:status` carries status events from Processor back to API.
- **Rationale:** Streams provide built-in consumer group semantics (at-least-once delivery), pending entry list (PEL) for crash recovery via `XAUTOCLAIM`, and per-message acknowledgement. No separate broker dependency beyond Redis which is already required.
- **Tradeoff accepted:** Low-priority messages can starve under sustained high load. Acceptable for this scope.

### ADR-2: Redis Token Bucket for Rate Limiting
- **Decision:** Lua script executed atomically in Redis. Key pattern: `ratelimit:{channel}`. Capacity: 100 tokens. Refill: 100/s. Burst allowance: 120 (20% headroom).
- **Rationale:** Distributed-safe — works correctly across multiple Processor instances. Survives app restarts.
- **Tradeoff accepted:** Adds Redis round-trip per notification dispatch. Negligible at this scale.

### ADR-3: Exponential Backoff with Jitter for Retries
- **Decision:** Failed deliveries are re-enqueued into the appropriate priority stream with `deliver_after` embedded in the message payload. The worker skips messages not yet due by putting them back (XACK + XADD with updated payload).
- **Formula:** `delay = min(base * 2^attempt, max_delay) + jitter` where jitter is random in `[0, delay * 0.2]`.
- **Tradeoff accepted:** Retry delays are approximate (worker poll interval adds latency). Acceptable.

### ADR-4: Dual Idempotency Strategy
- **Decision:** Check client-supplied `Idempotency-Key` header first (stored in Redis, 24h TTL). If absent, compute `sha256(channel + recipient + content)` and check against `idempotency_keys` table with 1h window.
- **Rationale:** Gives API consumers explicit control while protecting against accidental duplicates.
- **Tradeoff accepted:** Content hash collisions theoretically possible but negligible risk.

### ADR-5: Two-Service Architecture
- **Decision:** Single monorepo with two `cmd/` entrypoints — Notification Management API (owns PostgreSQL, exposes REST) and Notification Processor (consumes Redis Streams, performs delivery). Shared `internal/` packages for common types and infrastructure.
- **Rationale:** Allows independent scaling of ingestion vs. delivery. API instances can be scaled horizontally; Processor worker count tuned separately via `WORKER_CONCURRENCY`.
- **Tradeoff accepted:** Adds inter-service communication via Redis round-trips. Negligible at this scale.

### ADR-6: Event-Driven Status Updates (Processor → API)
- **Decision:** Processor publishes delivery outcomes to `notify:stream:status`. A consumer goroutine inside the API service reads this stream and writes `delivery_attempts` rows + updates `notifications.status` in PostgreSQL.
- **Rationale:** Processor does not need its own database. Status persistence is the API service's responsibility.
- **Tradeoff accepted:** Status updates are eventually consistent — there is a short lag between delivery and PostgreSQL reflecting the new status. Acceptable for this scope.

---

## Development Commands

| Purpose | Command |
|---------|---------|
| Build all | `go build ./...` |
| Vet | `go vet ./...` |
| Test all | `go test ./...` |
| Test with race detector | `go test -race ./...` |
| Lint | `golangci-lint run` |
| Generate API docs | `swag init -dir cmd/api` |
| Run migrations | `go run ./cmd/api migrate up` |
| Run API service | `go run ./cmd/api` |
| Run Processor | `go run ./cmd/processor` |
| Start full stack | `docker-compose up` |
