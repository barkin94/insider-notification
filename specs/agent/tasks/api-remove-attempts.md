# api-remove-attempts

**Specs:** `system/TODOS.md` § Processor Database, `api-service/DATA_MODEL.md`, `api-service/API_CONTRACT.md`
**Depends on:** `processor-worker-attempts`
**Status:** pending

## Context

With the processor writing `delivery_attempts` directly, the API service no longer needs
the `delivery_attempts` table, its repository, or the `StatusConsumer`'s attempt write.
`GET /notifications/:id` stops returning attempt history — that data now lives in the
processor's schema and the API has no business querying it.

## What to remove

### `api/migrations/`
- Delete `000002_create_delivery_attempts.up.sql` and `000002_create_delivery_attempts.down.sql`

If the migration runner applies migrations by filename order, verify that removing migration
002 does not break existing deployments. For this Docker Compose scope: bring the stack
down, remove the postgres volume, and bring it back up cleanly.

### `api/internal/db/delivery_attempt_repo.go` and `delivery_attempt_repo_test.go`
Delete both files entirely.

### `api/internal/db/repos.go`
Remove the `DeliveryAttemptRepository` interface.

### `api/internal/consumer/status.go`
- Remove `attemptRepo db.DeliveryAttemptRepository` field and `NewStatusConsumer` parameter
- Remove the `attempt := &model.DeliveryAttempt{...}` construction and `c.attemptRepo.Create(...)` call
- Keep only `c.notifRepo.UpdateStatus(ctx, notifID, evt.Status)` and its surrounding logic

### `api/internal/service/notification.go`
- Remove `attempts db.DeliveryAttemptRepository` field and `NewNotificationService` parameter
- Change `GetByID` signature: `GetByID(ctx, id) (*model.Notification, error)` — drop the `[]*model.DeliveryAttempt` return value
- Remove the `s.attempts.ListByNotificationID` call inside `GetByID`

### `api/internal/service/notification.go` — `NotificationService` interface
Update `GetByID` signature to match above.

### `api/internal/handler/notification.go`
- Remove `delivery_attempts` field from the `notificationResponse` struct
- Remove `toAttemptResponses` helper and the `attempts` parameter from the response builder
- Update the `GetByID` call to match the new single-return signature

### `api/cmd/main.go`
- Remove `db.NewDeliveryAttemptRepository(bundb)` construction
- Remove it from `NewStatusConsumer` and `NewNotificationService` call sites

## Spec files to update

### `specs/api-service/DATA_MODEL.md`
- Remove the `delivery_attempts` table definition entirely

### `specs/api-service/API_CONTRACT.md`
- Remove the `delivery_attempts` array from the `GET /notifications/:id` response shape

### `specs/processor-service/QUEUE_DESIGN.md`
- Update the status event consumer section: note it no longer writes `delivery_attempts`,
  only calls `UpdateStatus` on the `notifications` table

### `specs/api-service/VERIFICATION.md`
- Remove all checklist items that reference `delivery_attempts` in the API context

## Tests

Update any test that passed `attemptRepo` or expected `delivery_attempts` in the response:
- `api/internal/consumer/status_test.go` — remove `attemptRepo` setup and assertions
- `api/internal/service/notification_test.go` — update `GetByID` mock/call sites
- `api/internal/handler/notification_test.go` — remove `delivery_attempts` from expected response

## Verification

- `go test ./api/...` passes with no references to `DeliveryAttemptRepository`
- `grep -r "DeliveryAttempt\|delivery_attempt" api/` returns no matches outside of deleted files
- `GET /notifications/:id` response no longer contains `delivery_attempts` field
- `docker compose up --build` stack starts cleanly with fresh volumes
