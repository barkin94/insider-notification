# TASKS — Implementation Checklist

Use this file as your Cursor session driver. Each task references the relevant spec.
Check off items as you complete them. Start each Cursor session by pointing it at the
relevant spec files listed under each task.

---

## Phase 1 — Scaffold & Infrastructure (Est. 1.5h)

- [ ] **1.1** Initialize Go module (`go mod init`)
      → No spec dependency

- [ ] **1.2** Create project directory structure
      → See ARCHITECTURE.md §Project Layout

- [ ] **1.3** Write `docker-compose.yml`
      → Specs: PRD.md §Constraints, ARCHITECTURE.md §Tech Stack
      Components: app, mongo:7, redis:7-alpine
      - mongo: port 27017, volume for data, healthcheck, replica set init (required for transactions)
      - redis: port 6379, `--appendonly yes`, volume for data
      - app: depends_on mongo + redis, env vars from `.env`

- [ ] **1.4** Write `Dockerfile`
      → Specs: ARCHITECTURE.md §Tech Stack, §Project Layout
      - Multi-stage: builder (go build) + runtime (distroless/scratch)
      - Expose port 8080

- [ ] **1.5** Write Go migration runner and initial migration functions
      → See DATA_MODEL.md §Migration Runner
      Migration functions (in internal/db/migrations/migrations.go):
      - Version 1: create_notifications_indexes
      - Version 2: create_delivery_attempts_indexes
      - Version 3: create_templates_indexes
      - Version 4: create_idempotency_keys_ttl_index
      → See DATA_MODEL.md §Migration Runner for index definitions

- [ ] **1.6** Write `config/config.go` with Viper
      → Specs: PRD.md §Constraints, ARCHITECTURE.md §Tech Stack
      Env vars:
      ```
      MONGODB_URI          mongodb://localhost:27017/notifications?replicaSet=rs0
      REDIS_URL            redis://localhost:6379
      PORT                 8080
      WORKER_CONCURRENCY   10
      LOG_LEVEL            info
      WEBHOOK_URL          https://webhook.site/{your-uuid}
      ```

- [ ] **1.7** Write `Makefile`
      → Specs: TASKS.md §Per-Task Check Commands (from .cursorrules)
      Targets: `run`, `test`, `migrate-up`, `migrate-down`, `generate-docs`, `lint`

---

## Phase 2 — Data Layer (Est. 1h)

- [ ] **2.1** Write domain model structs in `internal/model/`
      → Specs: DATA_MODEL.md §All Collections, §Status Transition Map, §Content Validation Rules
      Files: `notification.go`, `delivery_attempt.go`, `template.go`, `idempotency_key.go`
      Include: struct definitions, status constants, validation methods

- [ ] **2.2** Write MongoDB repository for notifications
      `internal/db/notification_repo.go`
      → Specs: DATA_MODEL.md §Collection: notifications, §Atomic Update Patterns, §Redis Key Patterns
      Methods:
      - `Create(ctx, notification) error`
      - `CreateBatch(ctx, []notification) ([]Result, error)`
      - `GetByID(ctx, id) (*Notification, error)`
      - `List(ctx, filters, pagination) ([]Notification, int, error)`
      - `UpdateStatus(ctx, id, status) error`
      - `UpdateAfterDelivery(ctx, id, providerMsgID, status) error`
      - `SetDeliverAfter(ctx, id, deliverAfter, attempts) error`
      - `GetDueRetries(ctx) ([]Notification, error)`
      - `GetDueScheduled(ctx) ([]Notification, error)`
      - `ReconcileStuckProcessing(ctx) ([]Notification, error)`

- [ ] **2.3** Write MongoDB repository for delivery attempts
      `internal/db/delivery_attempt_repo.go`
      → Specs: DATA_MODEL.md §Collection: delivery_attempts
      Methods:
      - `Create(ctx, attempt) error`
      - `GetByNotificationID(ctx, notificationID) ([]DeliveryAttempt, error)`

- [ ] **2.4** Write MongoDB repository for templates
      `internal/db/template_repo.go`
      → Specs: DATA_MODEL.md §Collection: templates
      Methods:
      - `Create(ctx, template) error`
      - `GetByID(ctx, id) (*Template, error)`
      - `List(ctx, filters, pagination) ([]Template, int, error)`

- [ ] **2.5** Write idempotency repository + Redis fast path
      `internal/idempotency/`
      → See DATA_MODEL.md §Collection: idempotency_keys, §Redis Key Patterns
      → See ARCHITECTURE.md §ADR-4
      Methods:
      - `Check(ctx, key) (*NotificationID, error)`
      - `Store(ctx, key, notificationID, keyType, ttl) error`
      - `ContentHash(channel, recipient, content) string`
      - `Cleanup(ctx) error`  ← deletes expired rows

---

## Phase 3 — Queue & Rate Limiter (Est. 1h)

- [ ] **3.1** Write Redis queue producer
      `internal/queue/producer.go`
      → See QUEUE_DESIGN.md §Enqueue Operation
      Methods:
      - `Enqueue(ctx, notificationID, priority) error`

- [ ] **3.2** Write Redis queue consumer + worker pool
      `internal/queue/consumer.go`
      → See QUEUE_DESIGN.md §Worker Poll Algorithm, §Worker Pool
      Methods:
      - `StartWorkers(ctx, concurrency int, handler WorkerFunc)`
      - `PollNext(ctx) (*string, error)`

- [ ] **3.3** Write Redis token bucket rate limiter
      `internal/ratelimit/token_bucket.go`
      → See ARCHITECTURE.md §ADR-2
      - Implement as atomic Lua script
      - `Allow(ctx, channel) (bool, error)`
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
      → See PRD.md §External Provider Integration
      - `Deliver(ctx, notification) (*ProviderResponse, error)`
      - Sets `X-Correlation-ID` header on outbound request
      - Parses `{ messageId, status, timestamp }` from 202 response
      - Returns typed error with HTTP status for retry eligibility check

- [ ] **4.2** Write retry logic
      `internal/retry/retry.go`
      → See RETRY_POLICY.md
      - `IsRetryable(httpStatus int, err error) bool`
      - `ComputeDelay(attemptNumber int) time.Duration`
        Formula: `min(60 * 2^(attempt-1), 480) + jitter(0, delay*0.2)` seconds

- [ ] **4.3** Write main delivery worker orchestrator
      `internal/service/worker_service.go`
      → Specs: RETRY_POLICY.md §Retry Worker Implementation, §Rate Limiter Interaction, DATA_MODEL.md §Atomic Update Patterns, QUEUE_DESIGN.md §Worker Pool
      - Ties together: queue consumer, rate limiter, delivery client, retry logic
      - Acquires/releases Redis lock `notify:lock:{id}`
      - Updates notification status in MongoDB
      - Inserts delivery_attempt record
      - Publishes WebSocket update on status change
      - Emits metrics on each attempt

- [ ] **4.4** Write retry polling worker
      `internal/retry/retry_worker.go`
      → Specs: RETRY_POLICY.md §Retry Worker Implementation, QUEUE_DESIGN.md §Enqueue Operation
      - Polls `GetDueRetries()` every 1 second
      - Re-enqueues due retries into their priority queue
      → See RETRY_POLICY.md §Retry Worker Implementation

- [ ] **4.5** Write startup reconciliation
      `internal/service/reconcile.go`
      → See QUEUE_DESIGN.md §Persistence & Durability
      - Runs once on startup before workers start
      - Re-enqueues `processing` notifications older than 2 minutes

---

## Phase 5 — API Handlers (Est. 1.5h)

- [ ] **5.1** Setup `chi` router with middleware
      `internal/api/router.go`
      → Specs: OBSERVABILITY.md §Correlation ID Middleware, §Structured Logging
      Middleware stack (in order):
      1. Request logger (zap)
      2. Correlation ID injector
      3. Recoverer (panic → 500)
      4. Timeout (30s per request)

- [ ] **5.2** Implement `POST /notifications` handler
      `internal/api/handler/notification_handler.go`
      → See API_CONTRACT.md §POST /notifications
      Flow:
      1. Parse + validate request body
      2. Validate content length per channel
      3. Resolve template if template_id provided
      4. Check idempotency (Redis fast path → DB fallback)
      5. Create notification in DB
      6. Store idempotency key
      7. If no scheduled_at: enqueue immediately
      8. Return 201

- [ ] **5.3** Implement `POST /notifications/batch` handler
      → Specs: API_CONTRACT.md §POST /notifications/batch, DATA_MODEL.md §Collection: notifications, §Atomic Update Patterns
      - Generate batch_id UUID
      - Process each notification independently
      - Collect results, return 207

- [ ] **5.4** Implement `GET /notifications/:id` handler
      → Specs: API_CONTRACT.md §GET /notifications/:id, DATA_MODEL.md §Collection: delivery_attempts
      - Fetch notification + delivery_attempts

- [ ] **5.5** Implement `GET /notifications` handler
      → Specs: API_CONTRACT.md §GET /notifications, DATA_MODEL.md §Collection: notifications
      - Build dynamic WHERE clause from query params
      - Pagination with total count

- [ ] **5.6** Implement `POST /notifications/:id/cancel` handler
      → Specs: API_CONTRACT.md §POST /notifications/:id/cancel, DATA_MODEL.md §Status Transition Map, §Atomic Update Patterns
      - Status transition validation
      - Remove from Redis queue if present (best effort)

- [ ] **5.7** Implement template handlers
      → Specs: API_CONTRACT.md §POST /templates, §GET /templates/:id, §GET /templates, DATA_MODEL.md §Collection: templates

- [ ] **5.8** Implement `GET /metrics` handler
      → See API_CONTRACT.md §GET /metrics
      → See OBSERVABILITY.md §Metrics

- [ ] **5.9** Implement `GET /health` handler
      → See API_CONTRACT.md §GET /health
      → See OBSERVABILITY.md §Health Check

---

## Phase 6 — WebSocket (Est. 0.5h)

- [ ] **6.1** Write WebSocket hub
      `internal/api/ws/hub.go`
      → Specs: ARCHITECTURE.md §ADR-5, API_CONTRACT.md §WS /ws/status/:notification_id
      - `Register(notificationID, conn)`
      - `Unregister(notificationID, conn)`
      - `Broadcast(notificationID, StatusUpdate)`
      - Hub runs in a single goroutine with channel-based communication

- [ ] **6.2** Write WebSocket handler
      `internal/api/ws/handler.go`
      → See API_CONTRACT.md §WS /ws/status/:notification_id
      - Upgrade HTTP → WebSocket
      - Send current status on connect
      - Register with hub
      - Ping/pong keepalive every 30s
      - Unregister + close on terminal status or disconnect

- [ ] **6.3** Wire hub.Broadcast() calls into worker_service.go status transitions
      → Specs: DATA_MODEL.md §Status Transition Map, ARCHITECTURE.md §ADR-5

---

## Phase 7 — Scheduler & Templates (Est. 0.5h)

- [ ] **7.1** Write scheduler worker
      `internal/scheduler/scheduler.go`
      → See ARCHITECTURE.md §ADR-6
      - Poll `GetDueScheduled()` every 5 seconds
      - Enqueue each result, update status to `pending`

- [ ] **7.2** Write template renderer
      `internal/template/renderer.go`
      → Specs: DATA_MODEL.md §Collection: templates, API_CONTRACT.md §POST /templates
      - `Render(content string, vars map[string]string) (string, error)`
      - Parse `{{variable_name}}` using `regexp.MustCompile(`\{\{(\w+)\}\}`)`
      - Return error if any variable in template is missing from vars
      - `ExtractVariables(content string) []string` — for template creation response

---

## Phase 8 — Tests (Est. 1.5h)

- [ ] **8.1** Unit tests: retry logic
      `internal/retry/retry_test.go`
      → Specs: RETRY_POLICY.md §Backoff Formula, §Non-Retryable Conditions
      Cases: delay formula correctness, jitter bounds, retryable status codes, non-retryable codes

- [ ] **8.2** Unit tests: token bucket rate limiter
      `internal/ratelimit/token_bucket_test.go`
      → Specs: ARCHITECTURE.md §ADR-2
      Cases: allows up to capacity, blocks at capacity, refills over time

- [ ] **8.3** Unit tests: template renderer
      `internal/template/renderer_test.go`
      → Specs: DATA_MODEL.md §Collection: templates, API_CONTRACT.md §POST /templates
      Cases: happy path, missing variable error, extra variables ignored, no variables

- [ ] **8.4** Unit tests: idempotency key resolution
      `internal/idempotency/idempotency_test.go`
      → Specs: ARCHITECTURE.md §ADR-4, DATA_MODEL.md §Collection: idempotency_keys, §Redis Key Patterns
      Cases: client key preferred, content hash fallback, 24h vs 1h TTL

- [ ] **8.5** Integration tests: notification API (requires DB + Redis)
      `internal/api/handler/notification_handler_test.go`
      → Specs: API_CONTRACT.md §All endpoints, DATA_MODEL.md §Status Transition Map
      Cases:
      - Create single notification → 201
      - Create with idempotency key → 409 on duplicate
      - Create batch → 207 with mixed results
      - Get by ID → 200 with delivery_attempts
      - List with filters → pagination works
      - Cancel pending → 200
      - Cancel delivered → 409

- [ ] **8.6** Integration test: end-to-end delivery flow
      `internal/service/worker_service_test.go`
      → Specs: RETRY_POLICY.md §Non-Retryable Conditions, §Attempt Limits, QUEUE_DESIGN.md §Worker Pool
      Cases:
      - Notification created → picked up by worker → delivered → status = delivered
      - Provider returns 500 → retry scheduled → eventually delivered
      - Provider returns 500 × 4 → status = failed
      - Provider returns 400 → immediately failed (no retry)

---

## Phase 9 — Docs & CI (Est. 0.5h)

- [ ] **9.1** Add `swaggo/swag` annotations to all handlers
      → Specs: API_CONTRACT.md §All endpoints
      Run `swag init` to generate `docs/` folder

- [ ] **9.2** Write `README.md`
      → Specs: ARCHITECTURE.md §Component Overview, §Key Design Decisions
      Sections:
      - Architecture overview (copy ASCII diagram from ARCHITECTURE.md)
      - Prerequisites (Docker, Go 1.2x)
      - Quick start (`docker-compose up`)
      - Running tests (`make test`)
      - API examples (curl snippets for key endpoints)
      - Design decisions (summarize ADRs)
      - Known tradeoffs

- [ ] **9.3** Write `.github/workflows/ci.yml`
      → Specs: PRD.md §Constraints (Go version)
      Jobs:
      - `lint`: `golangci-lint run`
      - `test`: spin up mongo + redis services, run `go test ./...`
      - `build`: `go build ./cmd/server`
      Trigger: push to main + PRs

---

## Done Criteria

- [ ] `docker-compose up` starts system with no errors
- [ ] `make test` passes all tests
- [ ] `GET /health` returns `{"status": "ok"}`
- [ ] `POST /notifications` → notification appears in DB with `pending` status
- [ ] Worker picks it up → status transitions to `delivered`
- [ ] `GET /metrics` shows correct queue depths and delivery counts
- [ ] WebSocket client receives status update in real-time
- [ ] Duplicate request with same `Idempotency-Key` → 409
- [ ] Swagger UI accessible at `http://localhost:8080/swagger`
- [ ] Commit history is clean and atomic (one commit per phase minimum)
