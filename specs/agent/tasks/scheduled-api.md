# scheduled-api

**Specs:** `api-service/DATA_MODEL.md`, `api-service/API_CONTRACT.md`, `system/TODOS.md` § Scheduled Notifications
**Verification:** `api-service/VERIFICATION.md`
**Status:** pending

## Context

Notifications created with a `deliver_after` at least 1 minute in the future are stored
with status `scheduled` and are not published to the queue at creation time. The scheduler
worker (next task) owns the `scheduled → pending` transition and enqueue.

No new DB column — `deliver_after` already exists and `idx_notifications_deliver_after_status`
already covers the scheduler's query. The only schema change is adding `scheduled` to the
status enum.

## What to build

### `shared/model/enums.go`

Add:
```go
StatusScheduled = "scheduled"
```

### `shared/model/enums_test.go`

Add test cases for `StatusScheduled` / `"scheduled"` alongside existing ones.

### `api/internal/db/repos.go`

Add to `NotificationRepository`:
```go
TransitionFromAny(ctx context.Context, id uuid.UUID, from []string, to string) (*model.Notification, error)
```

Needed by `Cancel` to cancel from either `pending` or `scheduled` in one round-trip.

### `api/internal/db/notification_repo.go`

Implement `TransitionFromAny`:
```sql
UPDATE notifications
SET status = ?, updated_at = NOW()
WHERE id = ? AND status = ANY(?)
RETURNING *
```
Returns `db.ErrTransitionFailed` (same sentinel as `Transition`) when no row is updated.

### `api/internal/service/notification.go`

**`validate`:** if `req.DeliverAfter` is non-nil and not at least 1 minute in the future,
return `&ValidationError{Field: "deliver_after", Message: "must be at least 1 minute in the future"}`.

**`Create` and `createWithBatchID`:** determine status before persisting:
```go
status := model.StatusPending
if req.DeliverAfter != nil && req.DeliverAfter.After(time.Now().Add(time.Minute)) {
    status = model.StatusScheduled
}
n.Status = status

if err := s.repo.Create(ctx, n); err != nil { ... }

if status == model.StatusScheduled {
    return n, nil  // do not publish; scheduler will enqueue when due
}
// existing publish logic unchanged
```

**`Cancel`:** replace `Transition(id, StatusPending, StatusCancelled)` with:
```go
s.repo.TransitionFromAny(ctx, id, []string{model.StatusPending, model.StatusScheduled}, model.StatusCancelled)
```

## Spec files to update

### `specs/api-service/DATA_MODEL.md`
- Add `scheduled` to the status enum
- Add `scheduled → pending` and `scheduled → cancelled` to the status transition diagram

### `specs/api-service/API_CONTRACT.md`
- Add `deliver_after` validation rule to `POST /notifications`: must be ≥ 1 minute in the future if provided
- Add `scheduled` to the `status` filter values on `GET /notifications`
- Add `scheduled` to the cancellable statuses on `DELETE /notifications/:id/cancel`

## Tests

`api/internal/service/notification_test.go`:
- `TestCreate_futureDeliverAfter` — `deliver_after` = now+2min → `status = scheduled`, publisher not called
- `TestCreate_deliverAfterTooSoon` — `deliver_after` = now+30s → 422 validation error
- `TestCreate_noDeliverAfter` — existing behaviour unchanged: `status = pending`, publisher called
- `TestCancel_fromScheduled` — cancels a `scheduled` notification successfully

`api/internal/db/notification_repo_test.go`:
- `TestTransitionFromAny_pending` — row in `pending` → transitions to `cancelled`
- `TestTransitionFromAny_scheduled` — row in `scheduled` → transitions to `cancelled`
- `TestTransitionFromAny_noMatch` — row in `delivered` → returns `ErrTransitionFailed`

## Verification

- `go test ./shared/model/... ./api/internal/...` passes
- `POST /notifications` with `deliver_after` = now+2min → 201, `"status": "scheduled"`, nothing published to stream
- `POST /notifications` with `deliver_after` = now+30s → 422
- `DELETE /notifications/:id/cancel` on a `scheduled` notification → 200
