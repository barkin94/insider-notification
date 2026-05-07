# postgres-repos

**Specs:** `api-service/DATA_MODEL.md`, `system/ARCHITECTURE.md`
**Verification:** `api-service/VERIFICATION.md` § Data Layer
**Status:** pending

## What to build

### `internal/shared/db/pool.go`
```
NewPool(ctx, databaseURL string) (*pgxpool.Pool, error)
```

### `internal/shared/db/notification_repo.go`
```
NotificationRepository interface:
  Create(ctx, *model.Notification) error
  GetByID(ctx, uuid.UUID) (*model.Notification, error)
  List(ctx, ListFilter) ([]*model.Notification, int, error)
  Transition(ctx, id uuid.UUID, from, to string) (*model.Notification, error)
  IncrementAttempts(ctx, uuid.UUID) error

ListFilter struct:
  Status, Channel string; BatchID *uuid.UUID
  DateFrom, DateTo *time.Time
  Page, PageSize int; Sort, Order string

pgxNotificationRepository struct{ pool *pgxpool.Pool }  — implements the interface
```

### `internal/shared/db/delivery_attempt_repo.go`
```
DeliveryAttemptRepository interface:
  Create(ctx, *model.DeliveryAttempt) error  ← ON CONFLICT DO NOTHING

pgxDeliveryAttemptRepository — implements the interface
```

### `internal/shared/db/idempotency_repo.go`
```
IdempotencyRepository interface:
  GetByKey(ctx, key string) (*model.IdempotencyKey, error)
  Create(ctx, *model.IdempotencyKey) error
  DeleteExpired(ctx) error

pgxIdempotencyRepository — implements the interface
```

## Tests

`internal/shared/db/*_test.go` — testcontainers-go spins up PostgreSQL, runs migrations, then:

- `TestNotificationRepo_Create` — insert + GetByID round-trip
- `TestNotificationRepo_List_filters` — filter by status, channel, batch_id, date range
- `TestNotificationRepo_List_pagination` — page/page_size respected
- `TestNotificationRepo_Transition` — valid from→to succeeds; wrong from→ returns error
- `TestNotificationRepo_IncrementAttempts` — attempts field increments atomically
- `TestDeliveryAttemptRepo_Create_idempotent` — duplicate (notification_id, attempt_number) → no error, no duplicate row
- `TestIdempotencyRepo_GetByKey_expired` — expired key not returned
- `TestIdempotencyRepo_DeleteExpired` — only expired rows removed
