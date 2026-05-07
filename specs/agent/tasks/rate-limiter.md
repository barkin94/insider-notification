# rate-limiter

**Specs:** `system/ARCHITECTURE.md` (ADR: Redis Token Bucket), `processor-service/RETRY_POLICY.md`
**Verification:** `processor-service/VERIFICATION.md` § Rate Limiter
**Status:** pending

## What to build

### `internal/processor/ratelimit/limiter.go`
```
Limiter interface:
  Allow(ctx, channel string) (bool, error)

redisLimiter struct{ client *redis.Client; script *redis.Script }

NewLimiter(client *redis.Client) *redisLimiter
  — embeds Lua script at init; loads with SCRIPT LOAD

Lua script (atomic token bucket):
  key = "ratelimit:{channel}"
  capacity = 100, refill_rate = 100/s, burst = 120
  — read tokens + last_refill from hash
  — compute elapsed → add tokens (capped at burst)
  — if tokens >= 1: decrement, return 1 (allowed)
  — else: return 0 (denied)
  — write updated tokens + last_refill
```

## Tests

testcontainers-go (real Redis):

- `TestLimiter_allows` — first N requests within capacity → all allowed
- `TestLimiter_throttles` — burst exceeded → Allow returns false
- `TestLimiter_refills` — wait for refill window → allowed again
- `TestLimiter_atomic` — 50 goroutines call Allow concurrently → no token count corruption
