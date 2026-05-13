# scheduled-api

**Specs:** `api-service/DATA_MODEL.md`, `api-service/API_CONTRACT.md`
**Verification:** `api-service/VERIFICATION.md`
**Status:** pending

## Context

Notifications may carry a `deliver_after` timestamp. The API has no scheduling concern:
it always stores the notification as `pending` and publishes to the queue immediately,
including `deliver_after` in the event payload. The processor scheduler handles timing.

There is no `scheduled` status. Notifications with a future `deliver_after` stay `pending`
until the processor delivers them. Cancel always transitions `pending → cancelled`.

## What to build

### Revert stale scheduled-status changes

**`shared/model/enums.go` / `enums_test.go`**
- Remove `StatusScheduled = "scheduled"` and its test cases.

**`api/internal/db/repos.go`**
- Remove `TransitionFromAny` from `NotificationRepository`.

**`api/internal/db/notification_repo.go`**
- Remove the `TransitionFromAny` implementation.

**`api/internal/db/notification_repo_test.go`**
- Remove `TestTransitionFromAny_pending`, `TestTransitionFromAny_scheduled`,
  `TestTransitionFromAny_noMatch`.

**`api/internal/service/notification.go`**
- Remove the deliver_after validation check from `validate()`.
- In `Create` and `createWithBatchID`: always use `StatusPending`; always publish;
  remove the `StatusScheduled` early-return branch.
- In `Cancel`: revert to `s.repo.Transition(ctx, id, model.StatusPending, model.StatusCancelled)`.

**`api/internal/service/notification_test.go`**
- Remove `transitionFromAnyFn` field and `TransitionFromAny` method from `mockNotifRepo`.
- Remove `TestCreate_futureDeliverAfter`, `TestCreate_deliverAfterTooSoon`,
  `TestCancel_fromScheduled`.
- Update `TestCancel_transitionFailed`: override `transitionFn` (not `transitionFromAnyFn`).

### `api/internal/handler/notification.go`

Add `deliver_after` to `createRequest`:

```go
type createRequest struct {
    Recipient    string          `json:"recipient"`
    Channel      string          `json:"channel"`
    Content      string          `json:"content"`
    Priority     string          `json:"priority"`
    Metadata     json.RawMessage `json:"metadata" swaggertype:"object"`
    DeliverAfter *string         `json:"deliver_after"` // RFC3339; omit for immediate delivery
}
```

In `createNotification`, parse before calling `svc.Create`:

```go
var deliverAfter *time.Time
if req.DeliverAfter != nil {
    t, err := time.Parse(time.RFC3339, *req.DeliverAfter)
    if err != nil {
        return errBadRequest("VALIDATION_ERROR", "deliver_after must be RFC3339")
    }
    deliverAfter = &t
}
```

Apply the same mapping in `createBatch`.

Add `deliver_after` to `notificationResponse` and populate it in `toNotificationResponse`:

```go
DeliverAfter *string `json:"deliver_after"`
```

## Tests

No new service or repo tests. Existing tests continue to pass unchanged.

## Verification

- `go test ./shared/model/... ./api/internal/...` passes
- `POST /notifications` with `deliver_after` → 201, `"status": "pending"`, event published with deliver_after in payload
- `POST /notifications` without `deliver_after` → 201, `"status": "pending"`, event published
- `DELETE /notifications/:id/cancel` → 200
