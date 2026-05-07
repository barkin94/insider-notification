# VERIFICATION — Notification Processor

---

## Stream Consumer & Worker Pool

- [ ] Consumer group `notify:cg:processor` created on all three priority streams on startup
- [ ] Worker poll order confirmed: high → normal → low before blocking
- [ ] 10 workers run concurrently (configurable via `WORKER_CONCURRENCY`)
- [ ] Redis lock acquired before processing; lock TTL is 60s
- [ ] Lock miss → message acknowledged, worker moves to next
- [ ] Notification fetched from PostgreSQL after lock acquired; cancelled/already-processed notifications skipped
- [ ] Status transition to `processing` is atomic; skip if another worker already transitioned
- [ ] Message acknowledged after every terminal outcome (success, failure, skip)
- [ ] Crash recovery: messages idle in PEL > 2 minutes are reclaimed on startup
- [ ] `go test ./internal/shared/stream/... ./processor/internal/worker/...` passes

---

## Rate Limiter

- [ ] Lua script executes atomically in Redis
- [ ] Token bucket capacity 100, refill rate 100/s, burst 120 per channel
- [ ] Rate-limited notification re-enqueued immediately (no backoff); attempt counter not incremented
- [ ] Worker sleeps `1000ms / capacity` (10ms) after a rate-limit hit
- [ ] Concurrent goroutine test confirms atomic execution
- [ ] `go test ./processor/internal/ratelimit/...` passes

---

## Retry

- [ ] Backoff formula: `min(60s * 2^(attempt-1), 480s) + jitter` where `jitter ∈ [0, delay * 0.2]`
- [ ] Computed delays match `RETRY_POLICY.md` table (attempt 2 ≈ 60–72s, attempt 3 ≈ 120–144s, attempt 4 ≈ 240–288s)
- [ ] `go test ./processor/internal/retry/...` passes

---

## Delivery Client

- [ ] HTTP POST sent to webhook.site with correct JSON body
- [ ] `X-Correlation-ID` header forwarded on every outbound call
- [ ] Provider 202 → success
- [ ] Provider 400 / 401 / 403 → non-retryable failure
- [ ] Provider 5xx / 429 / timeout → retryable failure
- [ ] Latency measured from dispatch to response
- [ ] `go test ./processor/internal/delivery/...` passes

---

## Worker End-to-End

- [ ] Successful delivery → status event `delivered` published to `notify:stream:status`
- [ ] Retryable failure with attempts < max → re-enqueued with `deliver_after` = backoff timestamp; status event `processing` published
- [ ] Non-retryable failure OR attempts == max → status event `failed` published; no re-enqueue
- [ ] `deliver_after` in the future → message re-enqueued immediately without consuming retry budget
- [ ] `go test ./processor/internal/worker/...` passes
