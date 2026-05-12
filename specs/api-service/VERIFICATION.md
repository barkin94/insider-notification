# VERIFICATION — Notification Management API

---

## Data Layer

- [ ] `go build ./...` and `go vet ./...` pass
- [ ] All struct fields match `DATA_MODEL.md` (field names, types, nullability)
- [ ] All three migrations apply cleanly via `golang-migrate`; `down` migrations reverse cleanly
- [ ] All PostgreSQL indexes from `DATA_MODEL.md` present in migration files
- [ ] `NotificationRepository` interface implemented
- [ ] `go test ./internal/shared/...` passes

---

## Stream Producer

- [ ] API publishes to `notify:stream:{priority}` after creating a notification
- [ ] Message fields match `MESSAGE_CONTRACT.md` (`notification_id`, `deliver_after`)
- [ ] Consumer group `notify:cg:api` created on `notify:stream:status` on startup
- [ ] `go test ./internal/shared/stream/...` passes (producer side)

---

## Middleware

- [ ] All log lines include `trace_id` and `span_id` from the active span (injected by OTel log bridge)
- [ ] `go test ./api/internal/middleware/...` passes

---

## HTTP Handlers

- [ ] All 6 endpoints from `API_CONTRACT.md` registered in router
- [ ] `POST /notifications` → 201; body matches `API_CONTRACT.md` response shape
- [ ] `POST /notifications/batch` → 207; rejected items include per-item error; accepted items have `id`
- [ ] `POST /notifications/batch` > 1000 items → 400
- [ ] `GET /notifications/:id` → 200 with notification fields
- [ ] `GET /notifications/:id` unknown ID → 404
- [ ] `GET /notifications` pagination fields match `API_CONTRACT.md` (`page_size`, `total`, `next_cursor`)
- [ ] `GET /notifications` filters (`status`, `channel`, `batch_id`, `date_from`, `date_to`) work correctly
- [ ] `POST /notifications/:id/cancel` on `pending` → 200 with `status: cancelled`
- [ ] `POST /notifications/:id/cancel` on `delivered`/`failed`/`cancelled` → 409 `INVALID_STATUS_TRANSITION`
- [ ] `GET /health` → 200 with `postgresql` and `redis` checks; 503 if either fails
- [ ] Content length enforced per channel (SMS 1600, Email 100000, Push 4096)
- [ ] `go test ./api/internal/handler/...` passes

---

## Status Event Consumer

- [ ] Consumer reads from `notify:stream:status` via consumer group `notify:cg:api`
- [ ] Updates `notifications.status` for `delivered` and `failed` events
- [ ] Acknowledges message after DB write completes
- [ ] Re-processing a duplicate status event is safe (idempotent via UpdateStatus)
- [ ] `go test ./api/internal/consumer/...` passes
