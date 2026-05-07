# idempotency

**Specs:** `api-service/DATA_MODEL.md`, `api-service/API_CONTRACT.md`
**Verification:** `api-service/VERIFICATION.md` § Idempotency
**Status:** complete

## What to build

### `internal/api/idempotency/service.go`
```
KeyStore interface:
  Get(ctx, key string) (uuid.UUID, error)
  Set(ctx, key string, id uuid.UUID, ttl time.Duration) error

Result struct:
  Duplicate       bool
  ExistingID      uuid.UUID

Service struct{ keys KeyStore; idempotencyRepo db.IdempotencyRepository }

Check(ctx, idempotencyKey string, n *model.Notification) (Result, error)
  1. If idempotencyKey non-empty:
     - GET idempotency:{idempotencyKey} from Redis
     - Hit → return Result{Duplicate: true, ExistingID: <stored UUID>}
     - Miss → return Result{Duplicate: false}
  2. If idempotencyKey empty:
     - Compute sha256(channel + recipient + content)
     - Query idempotency_keys table for hash where expires_at > now
     - Hit → return Result{Duplicate: true, ExistingID: record.NotificationID}
     - Miss → return Result{Duplicate: false}

Store(ctx, idempotencyKey string, n *model.Notification) error
  — called after notification is created; writes Redis key + DB row
```

### `internal/api/idempotency/redis_store.go`
```
redisKeyStore struct{ client *redis.Client }  — implements KeyStore
  — Get: GET idempotency:{key}; returns db.ErrNotFound on cache miss
  — Set: SET idempotency:{key} {id} EX {ttl}
```

```
Service.StartCleanup(ctx)
  — goroutine: ticker every 1h → idempotencyRepo.DeleteExpired(ctx)
```

## Tests

testcontainers-go (real Redis + PostgreSQL):

- `TestChecker_clientKey_duplicate` — same client key → Duplicate=true, ExistingID matches
- `TestChecker_clientKey_miss` — new client key → Duplicate=false
- `TestChecker_contentHash_duplicate` — same channel+recipient+content → Duplicate=true
- `TestChecker_contentHash_expired` — expired hash row → Duplicate=false (treated as new)
- `TestCleanup_deletesExpired` — expired rows gone after cleanup runs
