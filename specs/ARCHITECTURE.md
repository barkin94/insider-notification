# ARCHITECTURE — Notification System

## Component Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                          API Layer                              │
│   POST /notifications   GET /notifications/:id   WS /ws/status  │
│   POST /notifications/batch              GET /metrics            │
│   POST /notifications/:id/cancel        GET /health              │
│   POST /templates   GET /templates/:id                          │
└───────────────────────────┬─────────────────────────────────────┘
                            │
              ┌─────────────▼─────────────┐
              │     Idempotency Check      │
              │  (Redis + Postgres lookup) │
              └─────────────┬─────────────┘
                            │
              ┌─────────────▼─────────────┐
              │      Scheduler Service     │
              │  (persists to DB, enqueues │
              │   at scheduled_at time)    │
              └─────────────┬─────────────┘
                            │
         ┌──────────────────▼──────────────────┐
         │            Redis Queues              │
         │  notify:queue:high                   │
         │  notify:queue:normal                 │
         │  notify:queue:low                    │
         └──────────┬────────────┬─────────────┘
                    │            │
       ┌────────────▼──┐  ┌──────▼────────────┐
       │  Queue Workers │  │  Scheduler Worker  │
       │  (poll high    │  │  (polls DB for     │
       │   first)       │  │   scheduled_at <=  │
       └────────┬───────┘  │   NOW(), enqueues) │
                │           └───────────────────┘
                │
   ┌────────────▼────────────┐
   │    Rate Limiter          │
   │  Redis token bucket      │
   │  100 msg/s per channel   │
   └────────────┬────────────┘
                │
   ┌────────────▼────────────┐
   │   Delivery Service       │
   │  POST webhook.site/uuid  │
   └────────────┬────────────┘
                │
   ┌────────────▼────────────┐
   │   Retry Scheduler        │
   │  exponential backoff     │
   │  + jitter, max 4 tries   │
   └────────────┬────────────┘
                │
   ┌────────────▼────────────┐
   │   MongoDB                │
   │  notifications           │
   │  delivery_attempts       │
   │  templates               │
   │  idempotency_keys        │
   └─────────────────────────┘
                │
   ┌────────────▼────────────┐
   │   WebSocket Hub          │
   │  broadcasts status       │
   │  changes to subscribers  │
   └─────────────────────────┘
```

## Tech Stack

| Layer | Choice | Rationale |
|---|---|---|
| Language | Go 1.2x | Required by case study; excellent concurrency primitives |
| HTTP Framework | `net/http` + `chi` router | Lightweight, idiomatic Go |
| Database | MongoDB 7 (replica set) | Flexible schema, TTL indexes for idempotency cleanup, native BSON types, multi-document transactions for batch ops |
| Queue broker | Redis 7 (Lists + BRPOPLPUSH) | Low-latency, atomic list ops, native TTL for idempotency keys |
| Rate limiter | Redis token bucket | Distributed-safe, survives app restarts |
| Migrations | Custom Go runner | Versioned Go functions tracked in `schema_migrations` collection; no SQL files |
| WebSocket | `gorilla/websocket` | Production-grade, well-maintained |
| Logging | `zap` | Structured, high-performance, supports correlation IDs |
| Config | `viper` | Env + file config, 12-factor compatible |
| Testing | `testify` + `go test` | Standard Go testing with assertions |
| API Docs | `swaggo/swag` | Generates OpenAPI from Go annotations |
| CI/CD | GitHub Actions | Automated test + lint pipeline |

## Project Layout

```
/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── handler/          # HTTP handlers
│   │   ├── middleware/        # logging, correlation ID
│   │   └── ws/               # WebSocket hub + handler
│   ├── config/
│   ├── db/
│   │   └── migrations/       # versioned Go migration functions
│   ├── model/                # domain structs
│   ├── queue/                # Redis queue producer + consumer
│   ├── ratelimit/            # token bucket implementation
│   ├── scheduler/            # scheduled notification worker
│   ├── delivery/             # webhook.site HTTP client
│   ├── retry/                # retry logic + backoff
│   ├── template/             # template storage + rendering
│   ├── idempotency/          # key resolution + dedup
│   ├── metrics/              # in-memory metrics store
│   └── service/              # orchestration layer
├── specs/                    # this directory
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── .github/
│   └── workflows/
│       └── ci.yml
└── docs/                     # swag-generated OpenAPI output
```

## Key Design Decisions

### ADR-1: Redis Lists as Priority Queues
- **Decision:** Three separate Redis lists (`notify:queue:high`, `notify:queue:normal`, `notify:queue:low`). Workers use `BRPOPLPUSH` — always polling high first, falling back to normal, then low in a loop.
- **Rationale:** No separate message broker dependency. Atomic ops prevent double-processing. Simple to reason about.
- **Tradeoff accepted:** Low-priority messages can starve under sustained high load. Acceptable for this scope.

### ADR-2: Redis Token Bucket for Rate Limiting
- **Decision:** Lua script executed atomically in Redis. Key pattern: `ratelimit:{channel}`. Capacity: 100 tokens. Refill: 100/s. Burst allowance: 120 (20% headroom).
- **Rationale:** Distributed-safe — works correctly across multiple app instances. Survives app restarts.
- **Tradeoff accepted:** Adds Redis round-trip per notification dispatch. Negligible at this scale.

### ADR-3: Exponential Backoff with Jitter for Retries
- **Decision:** Failed deliveries are re-enqueued into the appropriate priority queue with a `deliver_after` timestamp. A retry worker polls for due retries.
- **Formula:** `delay = min(base * 2^attempt, max_delay) + jitter` where jitter is random in [0, delay * 0.2].
- **Tradeoff accepted:** Retry delays are approximate (worker poll interval adds latency). Acceptable.

### ADR-4: Dual Idempotency Strategy
- **Decision:** Check client-supplied `Idempotency-Key` header first (stored in Redis, 24h TTL). If absent, compute `sha256(channel + recipient + content)` and check against `idempotency_keys` collection with 1h window.
- **Rationale:** Gives API consumers explicit control while protecting against accidental duplicates.
- **Tradeoff accepted:** Content hash collisions theoretically possible but negligible risk.

### ADR-5: WebSocket Hub for Real-Time Updates
- **Decision:** Central in-process hub with per-notification-ID subscription rooms. Status changes (written to DB) also publish to the hub which broadcasts to subscribers.
- **Rationale:** Simple, no external pub/sub dependency for this scope.
- **Tradeoff accepted:** Hub is in-process — not horizontally scalable without Redis pub/sub adapter. Acceptable for Docker Compose scope.

### ADR-6: Scheduler as a Polling Worker
- **Decision:** A dedicated goroutine polls notifications collection where scheduled_at <= NOW() and status = 'scheduled' every 5 seconds, enqueues them, and updates status to `pending`.
- **Rationale:** Simple, no cron dependency. 5s granularity is sufficient for notification scheduling.
- **Tradeoff accepted:** Up to 5 second delivery delay for scheduled notifications.
