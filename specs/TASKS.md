# TASKS — Implementation Checklist

Use this file as your session driver. Each task references the relevant spec.
Check off items as you complete them. Start each session by pointing it at the
relevant spec files listed under each task.

Project-specific execution authority:

- `TASKS.md` is the deciding factor for project-specific implementation actions
- `TASKS.md` is the deciding factor for required per-task checks
- `TASKS.md` is the deciding factor for phase verification checklists
- If any project-specific instruction conflicts with `AGENT_EXECUTION_PROTOCOL.md`, follow `TASKS.md`

---

## Phase 1 — Scaffold & Infrastructure (Est. 1.5h)

- [ ] **1.1** Initialize Go module (`go mod init`)
      → No spec dependency

- [ ] **1.2** Create project directory structure
      → See [Project Layout](./ARCHITECTURE.md#project-layout)

- [ ] **1.3** Write `docker-compose.yml`
      → Specs: [Constraints](./PRD.md#constraints), [Tech Stack](./ARCHITECTURE.md#tech-stack)
      Components: app, mongo:7, redis:7-alpine
      - mongo: port 27017, volume for data, healthcheck, replica set init (required for transactions)
      - redis: port 6379, `--appendonly yes`, volume for data
      - app: depends_on mongo + redis, env vars from `.env`

- [ ] **1.4** Write `Dockerfile`
      → Specs: [Tech Stack](./ARCHITECTURE.md#tech-stack), [Project Layout](./ARCHITECTURE.md#project-layout)
      - Multi-stage: builder (go build) + runtime (distroless/scratch)
      - Expose port 8080

- [ ] **1.5** Write Go migration runner and initial migration functions
      → See [Migration Runner](./DATA_MODEL.md#migration-runner)
      Migration functions (in internal/db/migrations/migrations.go):
      - Version 1: create_notifications_indexes
      - Version 2: create_delivery_attempts_indexes
      - Version 3: create_templates_indexes
      - Version 4: create_idempotency_keys_ttl_index
      → See [Migration Runner](./DATA_MODEL.md#migration-runner) for index definitions

- [ ] **1.6** Write `config/config.go` with Viper
      → Specs: [Constraints](./PRD.md#constraints), [Tech Stack](./ARCHITECTURE.md#tech-stack)
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
      → Specs: [Per-Task Check Commands](./TASKS.md#per-task-check-commands) (from .cursorrules)
      Targets: `run`, `test`, `migrate-up`, `migrate-down`, `generate-docs`, `lint`

---

## Phase 2 — Data Layer (Est. 1h)

- [ ] **2.1** Write domain model structs in `internal/model/`
      → Specs: [All collections](./DATA_MODEL.md#collection-notifications), §Status Transition Map, §Content Validation Rules
      Files: `notification.go`, `delivery_attempt.go`, `template.go`, `idempotency_key.go`
      Include: struct definitions, status constants, validation methods

- [ ] **2.2** Write MongoDB repository for notifications
      `internal/db/notification_repo.go`
      → Specs: [Collection: notifications](./DATA_MODEL.md#collection-notifications), §Atomic Update Patterns, §Redis Key Patterns
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
      → Specs: [Collection: delivery_attempts](./DATA_MODEL.md#collection-delivery_attempts)
      Methods:
      - `Create(ctx, attempt) error`
      - `GetByNotificationID(ctx, notificationID) ([]DeliveryAttempt, error)`

- [ ] **2.4** Write MongoDB repository for templates
      `internal/db/template_repo.go`
      → Specs: [Collection: templates](./DATA_MODEL.md#collection-templates)
      Methods:
      - `Create(ctx, template) error`
      - `GetByID(ctx, id) (*Template, error)`
      - `List(ctx, filters, pagination) ([]Template, int, error)`

- [ ] **2.5** Write idempotency repository + Redis fast path
      `internal/idempotency/`
      → See [Collection: idempotency_keys](./DATA_MODEL.md#collection-idempotency_keys), §Redis Key Patterns
      → See [ADR-4](./ARCHITECTURE.md#adr-4-dual-idempotency-strategy)
      Methods:
      - `Check(ctx, key) (*NotificationID, error)`
      - `Store(ctx, key, notificationID, keyType, ttl) error`
      - `ContentHash(channel, recipient, content) string`
      - `Cleanup(ctx) error`  ← deletes expired rows

---

## Phase 3 — Queue & Rate Limiter (Est. 1h)

- [ ] **3.1** Write Redis queue producer
      `internal/queue/producer.go`
      → See [Enqueue Operation](./QUEUE_DESIGN.md#enqueue-operation)
      Methods:
      - `Enqueue(ctx, notificationID, priority) error`

- [ ] **3.2** Write Redis queue consumer + worker pool
      `internal/queue/consumer.go`
      → See [Worker Poll Algorithm](./QUEUE_DESIGN.md#worker-poll-algorithm), §Worker Pool
      Methods:
      - `StartWorkers(ctx, concurrency int, handler WorkerFunc)`
      - `PollNext(ctx) (*string, error)`

- [ ] **3.3** Write Redis token bucket rate limiter
      `internal/ratelimit/token_bucket.go`
      → See [ADR-2](./ARCHITECTURE.md#adr-2-redis-token-bucket-for-rate-limiting)
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
      → See [External provider integration](./PRD.md#in-scope)
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
      → Specs: [Retry Worker Implementation](./RETRY_POLICY.md#retry-worker-implementation), §Rate Limiter Interaction, [Atomic Update Patterns](./DATA_MODEL.md#atomic-update-patterns), [Worker Pool](./QUEUE_DESIGN.md#worker-pool)
      - Ties together: queue consumer, rate limiter, delivery client, retry logic
      - Acquires/releases Redis lock `notify:lock:{id}`
      - Updates notification status in MongoDB
      - Inserts delivery_attempt record
      - Publishes WebSocket update on status change
      - Emits metrics on each attempt

- [ ] **4.4** Write retry polling worker
      `internal/retry/retry_worker.go`
      → Specs: [Retry Worker Implementation](./RETRY_POLICY.md#retry-worker-implementation), [Enqueue Operation](./QUEUE_DESIGN.md#enqueue-operation)
      - Polls `GetDueRetries()` every 1 second
      - Re-enqueues due retries into their priority queue
      → See [Retry Worker Implementation](./RETRY_POLICY.md#retry-worker-implementation)

- [ ] **4.5** Write startup reconciliation
      `internal/service/reconcile.go`
      → See [Persistence & Durability](./QUEUE_DESIGN.md#persistence--durability)
      - Runs once on startup before workers start
      - Re-enqueues `processing` notifications older than 2 minutes

---

## Phase 5 — API Handlers (Est. 1.5h)

- [ ] **5.1** Setup `chi` router with middleware
      `internal/api/router.go`
      → Specs: [Correlation ID Middleware](./OBSERVABILITY.md#correlation-id-middleware), §Structured Logging
      Middleware stack (in order):
      1. Request logger (zap)
      2. Correlation ID injector
      3. Recoverer (panic → 500)
      4. Timeout (30s per request)

- [ ] **5.2** Implement `POST /notifications` handler
      `internal/api/handler/notification_handler.go`
      → See [POST /notifications](./API_CONTRACT.md#post-notifications)
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
      → Specs: [POST /notifications/batch](./API_CONTRACT.md#post-notifications-batch), [Collection: notifications](./DATA_MODEL.md#collection-notifications), §Atomic Update Patterns
      - Generate batch_id UUID
      - Process each notification independently
      - Collect results, return 207

- [ ] **5.4** Implement `GET /notifications/:id` handler
      → Specs: [GET /notifications/:id](./API_CONTRACT.md#get-notifications-id), [Collection: delivery_attempts](./DATA_MODEL.md#collection-delivery_attempts)
      - Fetch notification + delivery_attempts

- [ ] **5.5** Implement `GET /notifications` handler
      → Specs: [GET /notifications](./API_CONTRACT.md#get-notifications), [Collection: notifications](./DATA_MODEL.md#collection-notifications)
      - Build dynamic WHERE clause from query params
      - Pagination with total count

- [ ] **5.6** Implement `POST /notifications/:id/cancel` handler
      → Specs: [POST /notifications/:id/cancel](./API_CONTRACT.md#post-notifications-id-cancel), [Status Transition Map](./DATA_MODEL.md#status-transition-map), §Atomic Update Patterns
      - Status transition validation
      - Remove from Redis queue if present (best effort)

- [ ] **5.7** Implement template handlers
      → Specs: [POST /templates](./API_CONTRACT.md#post-templates), §GET /templates/:id, §GET /templates, [Collection: templates](./DATA_MODEL.md#collection-templates)

- [ ] **5.8** Implement `GET /metrics` handler
      → See [GET /metrics](./API_CONTRACT.md#get-metrics)
      → See [Metrics](./OBSERVABILITY.md#metrics)

- [ ] **5.9** Implement `GET /health` handler
      → See [GET /health](./API_CONTRACT.md#get-health)
      → See [Health Check](./OBSERVABILITY.md#health-check)

---

## Phase 6 — WebSocket (Est. 0.5h)

- [ ] **6.1** Write WebSocket hub
      `internal/api/ws/hub.go`
      → Specs: [ADR-5](./ARCHITECTURE.md#adr-5-websocket-hub-for-real-time-updates), [WS /ws/status/:notification_id](./API_CONTRACT.md#ws-ws-status-notification_id)
      - `Register(notificationID, conn)`
      - `Unregister(notificationID, conn)`
      - `Broadcast(notificationID, StatusUpdate)`
      - Hub runs in a single goroutine with channel-based communication

- [ ] **6.2** Write WebSocket handler
      `internal/api/ws/handler.go`
      → See [WS /ws/status/:notification_id](./API_CONTRACT.md#ws-ws-status-notification_id)
      - Upgrade HTTP → WebSocket
      - Send current status on connect
      - Register with hub
      - Ping/pong keepalive every 30s
      - Unregister + close on terminal status or disconnect

- [ ] **6.3** Wire hub.Broadcast() calls into worker_service.go status transitions
      → Specs: [Status Transition Map](./DATA_MODEL.md#status-transition-map), [ADR-5](./ARCHITECTURE.md#adr-5-websocket-hub-for-real-time-updates)

---

## Phase 7 — Scheduler & Templates (Est. 0.5h)

- [ ] **7.1** Write scheduler worker
      `internal/scheduler/scheduler.go`
      → See [ADR-6](./ARCHITECTURE.md#adr-6-scheduler-as-a-polling-worker)
      - Poll `GetDueScheduled()` every 5 seconds
      - Enqueue each result, update status to `pending`

- [ ] **7.2** Write template renderer
      `internal/template/renderer.go`
      → Specs: [Collection: templates](./DATA_MODEL.md#collection-templates), [POST /templates](./API_CONTRACT.md#post-templates)
      - `Render(content string, vars map[string]string) (string, error)`
      - Parse `{{variable_name}}` using `regexp.MustCompile(`\{\{(\w+)\}\}`)`
      - Return error if any variable in template is missing from vars
      - `ExtractVariables(content string) []string` — for template creation response

---

## Phase 8 — Tests (Est. 1.5h)

- [ ] **8.1** Unit tests: retry logic
      `internal/retry/retry_test.go`
      → Specs: [Backoff Formula](./RETRY_POLICY.md#backoff-formula), §Non-Retryable Conditions
      Cases: delay formula correctness, jitter bounds, retryable status codes, non-retryable codes

- [ ] **8.2** Unit tests: token bucket rate limiter
      `internal/ratelimit/token_bucket_test.go`
      → Specs: [ADR-2](./ARCHITECTURE.md#adr-2-redis-token-bucket-for-rate-limiting)
      Cases: allows up to capacity, blocks at capacity, refills over time

- [ ] **8.3** Unit tests: template renderer
      `internal/template/renderer_test.go`
      → Specs: [Collection: templates](./DATA_MODEL.md#collection-templates), [POST /templates](./API_CONTRACT.md#post-templates)
      Cases: happy path, missing variable error, extra variables ignored, no variables

- [ ] **8.4** Unit tests: idempotency key resolution
      `internal/idempotency/idempotency_test.go`
      → Specs: [ADR-4](./ARCHITECTURE.md#adr-4-dual-idempotency-strategy), [Collection: idempotency_keys](./DATA_MODEL.md#collection-idempotency_keys), §Redis Key Patterns
      Cases: client key preferred, content hash fallback, 24h vs 1h TTL

- [ ] **8.5** Integration tests: notification API (requires DB + Redis)
      `internal/api/handler/notification_handler_test.go`
      → Specs: [Endpoints](./API_CONTRACT.md#endpoints), [Status Transition Map](./DATA_MODEL.md#status-transition-map)
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
      → Specs: [Non-Retryable Conditions](./RETRY_POLICY.md#non-retryable-conditions), §Attempt Limits, [Worker Pool](./QUEUE_DESIGN.md#worker-pool)
      Cases:
      - Notification created → picked up by worker → delivered → status = delivered
      - Provider returns 500 → retry scheduled → eventually delivered
      - Provider returns 500 × 4 → status = failed
      - Provider returns 400 → immediately failed (no retry)

---

## Phase 9 — Docs & CI (Est. 0.5h)

- [ ] **9.1** Add `swaggo/swag` annotations to all handlers
      → Specs: [Endpoints](./API_CONTRACT.md#endpoints)
      Run `swag init` to generate `docs/` folder

- [ ] **9.2** Write `README.md`
      → Specs: [Component Overview](./ARCHITECTURE.md#component-overview), §Key Design Decisions
      Sections:
      - Architecture overview (copy ASCII diagram from ARCHITECTURE.md)
      - Prerequisites (Docker, Go 1.2x)
      - Quick start (`docker-compose up`)
      - Running tests (`make test`)
      - API examples (curl snippets for key endpoints)
      - Design decisions (summarize ADRs)
      - Known tradeoffs

- [ ] **9.3** Write `.github/workflows/ci.yml`
      → Specs: [Constraints](./PRD.md#constraints) (Go version)
      Jobs:
      - `lint`: `golangci-lint run`
      - `test`: spin up mongo + redis services, run `go test ./...`
      - `build`: `go build ./cmd/server`
      Trigger: push to main + PRs

---

## Per-Task Check Commands

Use this deterministic format while executing each task.

Execution rules:

1. Run all checks under `always` whose `when` condition matches.
2. Run all checks under `task_specific` that match the current task ID.
3. A task is complete only if every required check exits with status code `0`.
4. Show full output for each executed check.

    always:
      - id: build-all
        when: any_go_file_changed
        run: go build ./...
      - id: vet-all
        when: any_go_file_changed
        run: go vet ./...

    task_specific:
      "1.3":
        - id: compose-config
          run: docker-compose config

      "1.5":
        - id: migrate-up
          run: go run ./cmd/migrate up
        - id: migrate-status
          run: go run ./cmd/migrate status

      "1.6":
        - id: config-build
          run: go build ./internal/config/...

      "2.x":
        - id: db-unit-tests
          run: go test ./internal/db/... -v -run TestUnit

      "3.1-3.2":
        - id: queue-tests
          run: go test ./internal/queue/... -v

      "3.3":
        - id: ratelimit-tests
          run: go test ./internal/ratelimit/... -v

      "4.x":
        - id: delivery-retry-service-tests
          run: go test ./internal/delivery/... ./internal/retry/... ./internal/service/... -v

      "5.x":
        - id: api-tests
          run: go test ./internal/api/... -v

      "6.x":
        - id: websocket-tests
          run: go test ./internal/api/ws/... -v

      "8.x":
        - id: all-tests
          run: go test ./... -v

      "9.1":
        - id: swagger-generate-and-list
          run: swag init && ls docs/

      "9.3":
        - id: ci-workflow-presence
          run: cat .github/workflows/ci.yml
          note: syntax check only

---

## Phase Verification Checklists

### Phase 1 — Scaffold

- [ ] docker-compose config passes with no errors
- [ ] go build ./... passes
- [ ] All directories from [Project Layout](./ARCHITECTURE.md#project-layout) exist
- [ ] .env.example file present with all vars from [Task 1.6 env vars](./TASKS.md#phase-1--scaffold--infrastructure-est-15h)
- [ ] Migration runner compiles and reports status cleanly

### Phase 2 — Data Layer

- [ ] go build ./... passes
- [ ] go vet ./... passes
- [ ] All repository methods from [Phase 2 tasks](./TASKS.md#phase-2--data-layer-est-1h) are implemented
- [ ] All struct fields match DATA_MODEL.md exactly (field names, bson tags, types)
- [ ] All MongoDB indexes from DATA_MODEL.md are created in migration Version 1-4
- [ ] Unit tests for repositories pass
- [ ] Idempotency: client key -> Redis fast path works, content hash -> DB fallback works

### Phase 3 — Queue & Rate Limiter

- [ ] go build ./... passes
- [ ] Queue producer LPUSH to correct priority list
- [ ] Worker poll order: high -> normal -> low confirmed in test
- [ ] Rate limiter Lua script executes atomically (test with concurrent goroutines)
- [ ] go test ./internal/queue/... ./internal/ratelimit/... passes

### Phase 4 — Delivery & Retry

- [ ] go build ./... passes
- [ ] Delivery client sends correct JSON shape to webhook.site
- [ ] Delivery client sets X-Correlation-ID header
- [ ] Retry: non-retryable codes (400, 401, 403) do NOT retry
- [ ] Retry: retryable codes (5xx, timeout) DO retry
- [ ] Backoff delay formula matches RETRY_POLICY.md exactly
- [ ] Jitter is within [0, delay * 0.2]
- [ ] Worker acquires Redis lock before processing
- [ ] Worker updates notification status in MongoDB after each transition
- [ ] go test ./internal/delivery/... ./internal/retry/... ./internal/service/... passes

### Phase 5 — API Handlers

- [ ] go build ./... passes
- [ ] All endpoints from API_CONTRACT.md are registered in router
- [ ] POST /notifications returns 201 with correct shape
- [ ] POST /notifications with duplicate idempotency key returns 409
- [ ] POST /notifications/batch returns 207 with per-item results
- [ ] GET /notifications/:id includes delivery_attempts array
- [ ] GET /notifications pagination fields match API_CONTRACT.md
- [ ] POST /notifications/:id/cancel returns 409 for delivered/failed
- [ ] GET /metrics returns all fields from [GET /metrics](./API_CONTRACT.md#get-metrics)
- [ ] GET /health returns 200 with mongodb + redis checks
- [ ] go test ./internal/api/... passes

### Phase 6 — WebSocket

- [ ] WebSocket connection upgrades successfully
- [ ] Current status sent immediately on connect
- [ ] Status updates broadcast within 1s of DB write
- [ ] Connection closes on terminal status (delivered/failed/cancelled)
- [ ] Ping/pong keepalive every 30s confirmed in code
- [ ] go test ./internal/api/ws/... passes

### Phase 7 — Scheduler & Templates

- [ ] Scheduler polls every 5s (configurable)
- [ ] Scheduled notifications enqueued within 5s of scheduled_at
- [ ] Template renderer resolves all {{variables}} correctly
- [ ] Missing variable returns error (not silent empty string)
- [ ] go test ./internal/scheduler/... ./internal/template/... passes

### Phase 8 — Tests

- [ ] go test ./... passes with zero failures
- [ ] Retry exhaustion test: 4 attempts -> status = failed
- [ ] Non-retryable test: 400 response -> status = failed immediately
- [ ] End-to-end test: notification created -> delivered -> status = delivered
- [ ] Race detector clean: go test -race ./...

### Phase 9 — Docs & CI

- [ ] docs/ directory generated by swag init
- [ ] Swagger UI accessible at /swagger/index.html after docker-compose up
- [ ] README.md has: architecture diagram, quick start, API examples, design decisions
- [ ] .github/workflows/ci.yml has lint, test, build jobs
- [ ] docker-compose up starts cleanly with no errors
- [ ] All TASKS.md Done Criteria checked off

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
