# idempotency

**Specs:** `api-service/DATA_MODEL.md`, `api-service/API_CONTRACT.md`
**Verification:** `api-service/VERIFICATION.md` § Idempotency
**Status:** pending

## What to build

### `internal/api/idempotency/checker.go`
```
Result struct:
  Duplicate       bool
  ExistingID      uuid.UUID

Checker struct{ redis *redis.Client; idempotencyRepo db.IdempotencyRepository }

Check(ctx, clientKey string, n *model.Notification) (Result, error)
  1. If clientKey non-empty:
     - GET idempotency:{clientKey} from Redis
     - Hit → return Result{Duplicate: true, ExistingID: <stored UUID>}
     - Miss → proceed to insert, then SET idempotency:{clientKey} = n.ID, TTL 24h
  2. If clientKey empty:
     - Compute sha256(channel + recipient + content)
     - Query idempotency_keys table for hash where expires_at > now
     - Hit → return Result{Duplicate: true, ExistingID: record.NotificationID}
     - Miss → insert row with key_type="content_hash", expires_at = now+1h

Store(ctx, clientKey string, id uuid.UUID) error
  — called after notification is created; writes Redis key + DB row
```

### `internal/api/idempotency/cleanup.go`
```
StartCleanup(ctx, repo db.IdempotencyRepository)
  — goroutine: ticker every 1h → repo.DeleteExpired(ctx)
```

## Tests

testcontainers-go (real Redis + PostgreSQL):

- `TestChecker_clientKey_duplicate` — same client key → Duplicate=true, ExistingID matches
- `TestChecker_clientKey_miss` — new client key → Duplicate=false
- `TestChecker_contentHash_duplicate` — same channel+recipient+content → Duplicate=true
- `TestChecker_contentHash_expired` — expired hash row → Duplicate=false (treated as new)
- `TestCleanup_deletesExpired` — expired rows gone after cleanup runs
