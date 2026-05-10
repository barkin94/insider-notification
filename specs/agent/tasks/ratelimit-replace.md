# ratelimit-replace

**Specs:** `system/ARCHITECTURE.md` (ADR: Redis Token Bucket)
**Status:** pending

## Context

The current `processor/internal/worker/ratelimit/limiter.go` uses an inline Lua script to implement
the token bucket atomically in Redis. This was noted as temporary — the Lua approach is hard to
read, test in isolation, and maintain.

## What to replace

### Current implementation
`redisLimiter` in `processor/internal/worker/ratelimit/limiter.go` — Lua script via `redis.NewScript`.

### Replacement options to evaluate

1. **go-redis built-in rate limiter** — `github.com/go-redis/redis_rate` uses a well-tested Lua
   script under the hood (GCRA algorithm). Hides the Lua; exposes a clean Go API.
   Tradeoff: still Lua internally, but not our maintenance burden.

2. **Redis Cell module** — `CL.THROTTLE` command (token bucket, no Lua). Requires Redis to be
   built with the Cell module. Tradeoff: external dependency on Redis build.

3. **Sliding window counter in Go** — use `INCR` + `EXPIRE` per window slot; combine with a
   Go-side `sync.Mutex` for the burst window. Tradeoff: not distributed-safe across multiple
   Processor instances without more coordination.

4. **github.com/mennanov/limiters** — pluggable rate limiter library with Redis backend, no
   custom Lua. Tradeoff: external dependency.

## Decision needed

Evaluate and pick one option before implementing. The replacement must:
- Preserve the `Limiter` interface (`Allow(ctx, channel string) (bool, error)`)
- Keep the same token bucket semantics: capacity=100, burst=120, refill=100/s
- Pass all existing tests in `processor/internal/worker/ratelimit/limiter_test.go`
- Contain no hand-written Lua scripts
