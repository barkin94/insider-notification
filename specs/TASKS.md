# TASKS

Project-specific authority: if anything here conflicts with `AGENT_INSTRUCTIONS.md`, follow this file.

Read at: session start and before each task.
After each task: consult `CHECKS.md`. At phase end: consult `VERIFICATION.md`.

---

## Phase 1 — Scaffold & Infrastructure (Est. 1.5h)

- [ ] **1.1** Initialize Go module (`go mod init`)
      → No spec dependency

- [ ] **1.2** Create project directory structure
      → [Project Layout](./ARCHITECTURE.md#project-layout)

- [ ] **1.3** Write `docker-compose.yml`
      → [Constraints](./PRD.md#constraints), [Tech Stack](./ARCHITECTURE.md#tech-stack)
      Components: app, mongo:7, redis:7-alpine
      - mongo: port 27017, volume, healthcheck, replica set init (required for transactions)
      - redis: port 6379, `--appendonly yes`, volume
      - app: depends_on mongo + redis, env vars from `.env`

- [ ] **1.4** Write `Dockerfile`
      → [Tech Stack](./ARCHITECTURE.md#tech-stack), [Project Layout](./ARCHITECTURE.md#project-layout)
      - Multi-stage: builder (go build) + runtime (distroless/scratch)
      - Expose port 8080

- [ ] **1.5** Write Go migration runner and initial migration functions
      → [Migration Runner](./DATA_MODEL.md#migration-runner)
      Functions in `internal/db/migrations/migrations.go`:
      - Version 1: create_notifications_indexes
      - Version 2: create_delivery_attempts_indexes
      - Version 3: create_templates_indexes
      - Version 4: create_idempotency_keys_ttl_index

- [ ] **1.6** Write `config/config.go` with Viper
      → [Constraints](./PRD.md#constraints), [Tech Stack](./ARCHITECTURE.md#tech-stack)
      Env vars: `MONGODB_URI`, `REDIS_URL`, `PORT` (8080), `WORKER_CONCURRENCY` (10), `LOG_LEVEL` (info), `WEBHOOK_URL`

- [ ] **1.7** Write `Makefile`
      → [CHECKS.md](./CHECKS.md)
      Targets: `run`, `test`, `migrate-up`, `migrate-down`, `generate-docs`, `lint`

---

## Phase 2 — Data Layer (Est. 1h)

- [ ] **2.1** Write domain model structs in `internal/model/`
      → [DATA_MODEL.md](./DATA_MODEL.md): all collections, Status Transition Map, Content Validation Rules
      Files: `notification.go`, `delivery_attempt.go`, `template.go`, `idempotency_key.go`

- [ ] **2.2** Write MongoDB repository for notifications
      `internal/db/notification_repo.go`
      → [Collection: notifications](./DATA_MODEL.md#collection-notifications), Atomic Update Patterns, Redis Key Patterns
      Methods: `Create`, `CreateBatch`, `GetByID`, `List`, `UpdateStatus`, `UpdateAfterDelivery`, `SetDeliverAfter`, `GetDueRetries`, `GetDueScheduled`, `ReconcileStuckProcessing`

- [ ] **2.3** Write MongoDB repository for delivery attempts
      `internal/db/delivery_attempt_repo.go`
      → [Collection: delivery_attempts](./DATA_MODEL.md#collection-delivery_attempts)
      Methods: `Create`, `GetByNotificationID`

- [ ] **2.4** Write MongoDB repository for templates
      `internal/db/template_repo.go`
      → [Collection: templates](./DATA_MODEL.md#collection-templates)
      Methods: `Create`, `GetByID`, `List`

- [ ] **2.5** Write idempotency repository + Redis fast path
      `internal/idempotency/`
      → [Collection: idempotency_keys](./DATA_MODEL.md#collection-idempotency_keys), Redis Key Patterns, [ADR-4](./ARCHITECTURE.md#adr-4-dual-idempotency-strategy)
      Methods: `Check`, `Store`, `ContentHash`, `Cleanup`

---

## Phase 3 — Queue & Rate Limiter (Est. 1h)

- [ ] **3.1** Write Redis queue producer
      `internal/queue/producer.go`
      → [Enqueue Operation](./QUEUE_DESIGN.md#enqueue-operation)
      Methods: `Enqueue(ctx, notificationID, priority) error`

- [ ] **3.2** Write Redis queue consumer + worker pool
      `internal/queue/consumer.go`
      → [Worker Poll Algorithm](./QUEUE_DESIGN.md#worker-poll-algorithm), Worker Pool
      Methods: `StartWorkers(ctx, concurrency, handler)`, `PollNext(ctx) (*string, error)`

- [ ] **3.3** Write Redis token bucket rate limiter
      `internal/ratelimit/token_bucket.go`
      → [ADR-2](./ARCHITECTURE.md#adr-2-redis-token-bucket-for-rate-limiting)
      - `Allow(ctx, channel) (bool, error)` — atomic Lua script
      - Capacity: 100, Burst: 120, Refill: 100/s
      Lua script:
      ```lua
      local key = KEYS[1]
      local capacity = tonumber(ARGV[1])
      local refill_rate = tonumber(ARGV[2])
      local now = tonumber(ARGV[3])
      local requested = tonumber(ARGV[4])
      local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
      local tokens = tonumber(bucket[1]) or capacity
      local last_refill = tonumber(bucket[2]) or now
      local elapsed = now - last_refill
      tokens = math.min(capacity, tokens + elapsed * refill_rate)
      if tokens >= requested then
        tokens = tokens - requested
        redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
        redis.call('EXPIRE', key, 60)
        return 1
      else
        redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
        redis.call('EXPIRE', key, 60)
        return 0
      end
      ```

---

## Phase 4 — Delivery & Retry (Est. 1.5h)

- [ ] **4.1** Write webhook.site delivery client
      `internal/delivery/provider.go`
      → [External provider integration](./PRD.md#in-scope)
      - `Deliver(ctx, notification) (*ProviderResponse, error)`
      - Sets `X-Correlation-ID` header; parses `{ messageId, status, timestamp }` from 202 response

- [ ] **4.2** Write retry logic
      `internal/retry/retry.go`
      → [RETRY_POLICY.md](./RETRY_POLICY.md)
      - `IsRetryable(httpStatus int, err error) bool`
      - `ComputeDelay(attempt int) time.Duration` — `min(60 * 2^(attempt-1), 480) + jitter(0, delay*0.2)` seconds

- [ ] **4.3** Write delivery worker orchestrator
      `internal/service/worker_service.go`
      → [RETRY_POLICY.md](./RETRY_POLICY.md): Retry Worker Implementation, Rate Limiter Interaction; [DATA_MODEL.md](./DATA_MODEL.md): Atomic Update Patterns; [QUEUE_DESIGN.md](./QUEUE_DESIGN.md): Worker Pool
      - Acquires/releases Redis lock `notify:lock:{id}`
      - Updates notification status in MongoDB
      - Inserts delivery_attempt record
      - Publishes WebSocket update on status change
      - Emits metrics on each attempt

- [ ] **4.4** Write retry polling worker
      `internal/retry/retry_worker.go`
      → [RETRY_POLICY.md](./RETRY_POLICY.md): Retry Worker Implementation; [QUEUE_DESIGN.md](./QUEUE_DESIGN.md): Enqueue Operation
      - Polls `GetDueRetries()` every 1 second, re-enqueues into priority queue

- [ ] **4.5** Write startup reconciliation
      `internal/service/reconcile.go`
      → [QUEUE_DESIGN.md](./QUEUE_DESIGN.md): Persistence & Durability
      - Runs once on startup; re-enqueues `processing` notifications older than 2 minutes

---

## Phase 5 — API Handlers (Est. 1.5h)

- [ ] **5.1** Setup `chi` router with middleware
      `internal/api/router.go`
      → [OBSERVABILITY.md](./OBSERVABILITY.md): Correlation ID Middleware, Structured Logging
      Middleware order: request logger (zap) → correlation ID → recoverer → timeout (30s)

- [ ] **5.2** Implement `POST /notifications`
      `internal/api/handler/notification_handler.go`
      → [API_CONTRACT.md](./API_CONTRACT.md#post-notifications)
      Flow: validate → resolve template → check idempotency → create in DB → store idempotency key → enqueue if no scheduled_at → 201

- [ ] **5.3** Implement `POST /notifications/batch`
      → [API_CONTRACT.md](./API_CONTRACT.md#post-notifications-batch), [DATA_MODEL.md](./DATA_MODEL.md): Atomic Update Patterns
      Generate batch_id UUID, process each independently, return 207

- [ ] **5.4** Implement `GET /notifications/:id`
      → [API_CONTRACT.md](./API_CONTRACT.md#get-notifications-id), [DATA_MODEL.md](./DATA_MODEL.md): delivery_attempts
      Fetch notification + delivery_attempts

- [ ] **5.5** Implement `GET /notifications`
      → [API_CONTRACT.md](./API_CONTRACT.md#get-notifications), [DATA_MODEL.md](./DATA_MODEL.md): notifications
      Dynamic filter from query params, pagination with total count

- [ ] **5.6** Implement `POST /notifications/:id/cancel`
      → [API_CONTRACT.md](./API_CONTRACT.md#post-notifications-id-cancel), [DATA_MODEL.md](./DATA_MODEL.md): Status Transition Map, Atomic Update Patterns
      Validate transition, remove from Redis queue (best effort)

- [ ] **5.7** Implement template handlers
      → [API_CONTRACT.md](./API_CONTRACT.md): POST /templates, GET /templates/:id, GET /templates; [DATA_MODEL.md](./DATA_MODEL.md): templates

- [ ] **5.8** Implement `GET /metrics`
      → [API_CONTRACT.md](./API_CONTRACT.md#get-metrics), [OBSERVABILITY.md](./OBSERVABILITY.md#metrics)

- [ ] **5.9** Implement `GET /health`
      → [API_CONTRACT.md](./API_CONTRACT.md#get-health), [OBSERVABILITY.md](./OBSERVABILITY.md#health-check)

---

## Phase 6 — WebSocket (Est. 0.5h)

- [ ] **6.1** Write WebSocket hub
      `internal/api/ws/hub.go`
      → [ADR-5](./ARCHITECTURE.md#adr-5-websocket-hub-for-real-time-updates), [API_CONTRACT.md](./API_CONTRACT.md#ws-ws-status-notification_id)
      Methods: `Register`, `Unregister`, `Broadcast` — hub runs in single goroutine via channels

- [ ] **6.2** Write WebSocket handler
      `internal/api/ws/handler.go`
      → [API_CONTRACT.md](./API_CONTRACT.md#ws-ws-status-notification_id)
      Upgrade → send current status → register → ping/pong every 30s → unregister on terminal status or disconnect

- [ ] **6.3** Wire `hub.Broadcast()` into worker_service.go status transitions
      → [DATA_MODEL.md](./DATA_MODEL.md): Status Transition Map, [ADR-5](./ARCHITECTURE.md#adr-5-websocket-hub-for-real-time-updates)

---

## Phase 7 — Scheduler & Templates (Est. 0.5h)

- [ ] **7.1** Write scheduler worker
      `internal/scheduler/scheduler.go`
      → [ADR-6](./ARCHITECTURE.md#adr-6-scheduler-as-a-polling-worker)
      Poll `GetDueScheduled()` every 5 seconds, enqueue, update status to `pending`

- [ ] **7.2** Write template renderer
      `internal/template/renderer.go`
      → [DATA_MODEL.md](./DATA_MODEL.md): templates, [API_CONTRACT.md](./API_CONTRACT.md#post-templates)
      - `Render(content string, vars map[string]string) (string, error)` — parse `{{variable_name}}` via `regexp.MustCompile(\{\{(\w+)\}\})`; error if variable missing
      - `ExtractVariables(content string) []string`

---

## Phase 8 — Tests (Est. 1.5h)

- [ ] **8.1** Unit tests: retry logic
      `internal/retry/retry_test.go`
      → [RETRY_POLICY.md](./RETRY_POLICY.md): Backoff Formula, Non-Retryable Conditions
      Cases: delay formula, jitter bounds, retryable codes, non-retryable codes

- [ ] **8.2** Unit tests: token bucket rate limiter
      `internal/ratelimit/token_bucket_test.go`
      → [ADR-2](./ARCHITECTURE.md#adr-2-redis-token-bucket-for-rate-limiting)
      Cases: allows up to capacity, blocks at capacity, refills over time

- [ ] **8.3** Unit tests: template renderer
      `internal/template/renderer_test.go`
      → [DATA_MODEL.md](./DATA_MODEL.md): templates, [API_CONTRACT.md](./API_CONTRACT.md#post-templates)
      Cases: happy path, missing variable error, extra variables ignored, no variables

- [ ] **8.4** Unit tests: idempotency key resolution
      `internal/idempotency/idempotency_test.go`
      → [ADR-4](./ARCHITECTURE.md#adr-4-dual-idempotency-strategy), [DATA_MODEL.md](./DATA_MODEL.md): idempotency_keys, Redis Key Patterns
      Cases: client key preferred, content hash fallback, 24h vs 1h TTL

- [ ] **8.5** Integration tests: notification API
      `internal/api/handler/notification_handler_test.go`
      → [API_CONTRACT.md](./API_CONTRACT.md): Endpoints; [DATA_MODEL.md](./DATA_MODEL.md): Status Transition Map
      Cases: create → 201, duplicate idempotency key → 409, batch → 207, get by ID with attempts, list pagination, cancel pending → 200, cancel delivered → 409

- [ ] **8.6** Integration test: end-to-end delivery flow
      `internal/service/worker_service_test.go`
      → [RETRY_POLICY.md](./RETRY_POLICY.md): Non-Retryable Conditions, Attempt Limits; [QUEUE_DESIGN.md](./QUEUE_DESIGN.md): Worker Pool
      Cases: created → delivered, 500 → retry → delivered, 500 × 4 → failed, 400 → immediately failed

---

## Phase 9 — Docs & CI (Est. 0.5h)

- [ ] **9.1** Add `swaggo/swag` annotations to all handlers
      → [API_CONTRACT.md](./API_CONTRACT.md): Endpoints
      Run `swag init` to generate `docs/`

- [ ] **9.2** Write `README.md`
      → [ARCHITECTURE.md](./ARCHITECTURE.md): Component Overview, Key Design Decisions
      Sections: architecture diagram, prerequisites, quick start, running tests, API examples, design decisions, known tradeoffs

- [ ] **9.3** Write `.github/workflows/ci.yml`
      → [PRD.md](./PRD.md): Constraints (Go version)
      Jobs: `lint` (golangci-lint), `test` (mongo + redis services, go test ./...), `build` (go build ./cmd/server)
      Trigger: push to main + PRs
