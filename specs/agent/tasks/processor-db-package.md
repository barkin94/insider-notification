# processor-db-package

**Specs:** `system/TODOS.md` § Processor Database
**Depends on:** `processor-postgres`
**Status:** pending

## Context

Builds the `processor/internal/db/` package and migrations, mirroring the structure of
`api/internal/db/`. The processor only needs `Create` on delivery attempts — nobody reads
them back through the processor service.

Note: the processor DSN sets `search_path=processor,public` so migrations and queries can
reference `delivery_attempts` without schema-prefixing.

## What to build

### `processor/migrations/000001_create_delivery_attempts.up.sql`

```sql
CREATE SCHEMA IF NOT EXISTS processor;

CREATE TABLE delivery_attempts (
    id               UUID        PRIMARY KEY,
    notification_id  UUID        NOT NULL,
    attempt_number   INT         NOT NULL,
    status           VARCHAR(20) NOT NULL,
    http_status_code INT,
    error_message    TEXT,
    latency_ms       INT,
    attempted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (notification_id, attempt_number)
);

CREATE INDEX idx_delivery_attempts_notification_id
    ON delivery_attempts(notification_id);
CREATE INDEX idx_delivery_attempts_attempted_at
    ON delivery_attempts(attempted_at DESC);
```

No FK to `notifications` — cross-schema foreign keys would couple the two services at the
DB level.

### `processor/migrations/000001_create_delivery_attempts.down.sql`
```sql
DROP TABLE IF EXISTS delivery_attempts;
DROP SCHEMA IF EXISTS processor;
```

### `processor/internal/db/open.go`

Identical to `api/internal/db/open.go` — `Open(databaseURL string) (*bun.DB, error)`.

### `processor/internal/db/errors.go`

```go
var ErrNotFound = errors.New("not found")
```

### `processor/internal/db/repos.go`
```go
// DeliveryAttemptRepository is the port for delivery attempt persistence.
type DeliveryAttemptRepository interface {
    Create(ctx context.Context, a *model.DeliveryAttempt) error
}
```

### `processor/internal/db/delivery_attempt_repo.go`
`bunDeliveryAttemptRepo` implementing the interface — identical shape to the API's
`delivery_attempt_repo.go`, using `ON CONFLICT (notification_id, attempt_number) DO NOTHING`.

### `processor/internal/db/migrate.go`

`RunMigrations(db *bun.DB) error` using `github.com/uptrace/bun/migrate` and an embedded FS
pointing at `processor/migrations/`.

### `processor/cmd/main.go`
After existing Redis/OTEL setup, open the DB and run migrations:
```go
bundb, err := processordb.Open(cfg.DatabaseURL)
// log and exit on err
if err := processordb.RunMigrations(bundb); err != nil {
    // log and exit
}
```

## Tests

`processor/internal/db/delivery_attempt_repo_test.go` using testcontainers-go (real postgres):

- `TestCreate_insertsRow` — creates a row, reads it back, checks fields match
- `TestCreate_idempotent` — same `(notification_id, attempt_number)` twice → no error, one row

## Verification

- `go test ./processor/internal/db/...` passes
- `docker compose up --build processor` starts; logs show migrations applied without error
- `psql "postgres://postgres:postgres@localhost:5432/notifications?options=-c search_path=processor" -c '\dt'`
  shows `delivery_attempts`
