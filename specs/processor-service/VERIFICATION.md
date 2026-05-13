# VERIFICATION — Notification Processor

---

## Stream Consumer & Worker Pool

- [ ] Consumer group `notify:cg:processor` created on all three priority streams on startup
- [ ] Worker poll order confirmed: high → normal → low before blocking
- [ ] 10 workers run concurrently (configurable via `WORKER_CONCURRENCY`)
- [ ] Cancellation checked before lock; cancelled → ACK and skip
- [ ] Redis lock acquired after cancellation check; lock TTL is 60s
- [ ] Lock miss → message acknowledged, worker moves to next
- [ ] Rate limit checked after lock; denied → re-enqueue and ACK (attempt counter not incremented)
- [ ] Message acknowledged after every terminal outcome (success, failure, skip)
- [ ] Crash recovery: messages idle in PEL > 2 minutes are reclaimed on startup
- [ ] `go test ./internal/shared/stream/... ./processor/internal/worker/...` passes

---

## Rate Limiter

- [ ] Lua script executes atomically in Redis
- [ ] Token bucket capacity 100, refill rate 100/s, burst 120 per channel
- [ ] Rate-limited notification re-enqueued immediately (no backoff); attempt counter not incremented
- [ ] Concurrent goroutine test confirms atomic execution
- [ ] `go test ./processor/internal/worker/ratelimit/...` passes

---

## Retry

- [ ] Backoff formula: `min(60s * 2^(attempt-1), 480s) + jitter` where `jitter ∈ [0, delay * 0.2]`
- [ ] Computed delays match `RETRY_POLICY.md` table (attempt 2 ≈ 60–72s, attempt 3 ≈ 120–144s, attempt 4 ≈ 240–288s)
- [ ] `go test ./processor/internal/worker/retry/...` passes

---

## Delivery Client

- [ ] HTTP POST sent to webhook.site with correct JSON body
- [ ] Provider 202 → success
- [ ] Provider 400 / 401 / 403 → non-retryable failure
- [ ] Provider 5xx / 429 / timeout → retryable failure
- [ ] Latency measured from dispatch to response
- [ ] `go test ./processor/internal/worker/webhook/...` passes

---

## Worker End-to-End

- [ ] Successful delivery → status event `delivered` published to `notify:stream:status`
- [ ] Retryable failure with attempts < max → re-enqueued with `deliver_after` = backoff timestamp; no status event published
- [ ] Non-retryable failure OR attempts == max → status event `failed` published; no re-enqueue
- [ ] `deliver_after` in the future → message ACKed and dropped; scheduler delivers when due
- [ ] `go test ./processor/internal/worker/...` passes
