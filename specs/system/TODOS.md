# TODOS

Features deferred from the initial implementation. Each entry includes enough
context to write a spec and implement when prioritized.

---

## Processor Database

**What:** Give the Notification Processor its own PostgreSQL database so it can write
`delivery_attempts` rows directly instead of publishing to `notify:stream:status` and
relying on the API service's status consumer.

**Motivation:**
- Removes coupling between Processor and API for delivery persistence
- Simplifies status updates (direct DB write vs. stream publish + consume)
- Required if the two services scale independently into separate infrastructure

**Design notes:**
- Processor would own `delivery_attempts` table; API service would read it via a shared
  read replica or a dedicated query API (not direct cross-DB joins)
- Alternatively: Processor owns its own PostgreSQL instance entirely, and the API service
  calls a Processor query endpoint for `delivery_attempts` on `GET /notifications/:id`
- The `notify:stream:status` stream and `notify:cg:api` consumer group would be removed
- Requires deciding on a cross-service data access pattern (read replica vs. gRPC query)

**Spec files to update when prioritized:**
- Add Processor PostgreSQL instance to diagram in `ARCHITECTURE.md`
- Add ADR for cross-service data access to `ARCHITECTURE.md`
- Add `delivery_attempts` table to a new `specs/processor-service/DATA_MODEL.md`
- Remove `notify:stream:status` and `notify:cg:api` from `QUEUE_DESIGN.md`
- Remove status event consumer section from `QUEUE_DESIGN.md`
- Update `VERIFICATION.md` to add Processor DB checks

---

## WebSocket: Real-Time Status Updates

**What:** A WebSocket endpoint that allows clients to subscribe to live status
changes for a specific notification.

**Endpoint:** `WS /ws/status/:notification_id`

**Behavior:**
- On connect: send current notification status immediately
- On each status change: broadcast new status to all subscribers of that notification ID
- Close connection automatically on terminal status (`delivered`, `failed`, `cancelled`)
- Ping/pong keepalive every 30 seconds

**Server → Client message shape:**
```json
{
  "notification_id": "uuid",
  "status": "delivered",
  "updated_at": "ISO8601",
  "attempt_number": 1
}
```

**Design notes:**
- In-process hub with per-notification-ID subscription rooms
- Status changes published to hub immediately after each DB write in the delivery worker
- Not horizontally scalable without a Redis pub/sub adapter (acceptable for Docker Compose scope)
- Requires `gorilla/websocket` dependency

**Spec files to update when prioritized:**
- Add `WS /ws/status/:notification_id` to `API_CONTRACT.md`
- Add WebSocket Hub ADR to `ARCHITECTURE.md`
- Add `api/internal/ws/` to project layout in `ARCHITECTURE.md`
- Add WebSocket section to `VERIFICATION.md`

---

## Scheduled Notifications

**What:** Allow notifications to be created with a future delivery time. A scheduler
worker polls for due notifications and enqueues them at the right time.

**Behavior:**
- `POST /notifications` accepts an optional `scheduled_at` ISO8601 field (must be at least 1 minute in future)
- Notifications with `scheduled_at` are stored with status `scheduled` instead of `pending`
- A dedicated goroutine polls for notifications where `scheduled_at <= NOW()` every 5 seconds,
  enqueues them into the priority queue, and transitions status to `pending`

**Design notes:**
- Adds `scheduled` as a new status value, cancellable via the cancel API
- Adds `scheduled_at` field to Notification struct and a sparse index
- Scheduler is a simple polling worker — no cron dependency; 5s granularity is acceptable
- Up to 5 second delivery delay is an accepted tradeoff

**Spec files to update when prioritized:**
- Add `scheduled_at` field and `scheduled` status to `DATA_MODEL.md`
- Add Scheduler Service and Scheduler Worker to diagram in `ARCHITECTURE.md`
- Add ADR for scheduler polling approach to `ARCHITECTURE.md`
- Add `internal/scheduler/` to project layout in `ARCHITECTURE.md`
- Add `scheduled_at` to `POST /notifications` and `GET /notifications` filter in `API_CONTRACT.md`
- Add `scheduled` to cancellable statuses in `API_CONTRACT.md`
- Add scheduler worker to "Called by" in `QUEUE_DESIGN.md`
- Add Scheduler & Templates section to `VERIFICATION.md`

---

## Template System

**What:** Pre-defined message templates with `{{variable}}` placeholders that are resolved
at notification creation time.

**Behavior:**
- Templates are stored in a separate PostgreSQL table with name, channel, content, and description
- `POST /notifications` accepts optional `template_id` + `template_vars` instead of `content`
- Template variables use `{{variable_name}}` syntax; all variables must be provided or request fails with 422
- Template CRUD: `POST /templates`, `GET /templates/:id`, `GET /templates`

**Design notes:**
- `content` and `template_id` are mutually exclusive on `POST /notifications`
- Variable extraction via `regexp.MustCompile(\{\{(\w+)\}\})`
- Missing variable returns `TEMPLATE_VAR_MISSING` (422), not a silent empty string

**Spec files to update when prioritized:**
- Add `Table: templates` to `DATA_MODEL.md`
- Add `TemplateID`, `TemplateVars` fields to Notification struct in `DATA_MODEL.md`
- Add migration file for templates table to `DATA_MODEL.md`
- Add `template_id`, `template_vars` to `POST /notifications` in `API_CONTRACT.md`
- Add `TEMPLATE_VAR_MISSING` error code to `API_CONTRACT.md`
- Add `POST /templates`, `GET /templates/:id`, `GET /templates` to `API_CONTRACT.md`
- Add `internal/template/` to project layout in `ARCHITECTURE.md`
- Add template renderer section to `VERIFICATION.md`

---

## GitHub Actions CI/CD

**What:** Automated pipeline that runs lint, tests, and build on every push to main and on PRs.

**Jobs:**
- `lint`: `golangci-lint run`
- `test`: spin up postgres + redis services, run `go test ./...`
- `build`: `go build ./cmd/server`

**Spec files to update when prioritized:**
- Add CI/CD row to Tech Stack table in `ARCHITECTURE.md`
- Add `.github/workflows/ci.yml` to project layout in `ARCHITECTURE.md`
- Add CI/CD checklist item to `VERIFICATION.md`

