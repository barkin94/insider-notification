# retry

**Specs:** `processor-service/RETRY_POLICY.md`
**Verification:** `processor-service/VERIFICATION.md` § Retry
**Status:** pending

## What to build

### `internal/processor/retry/backoff.go`
```
Delay(attempt int) time.Duration
  — formula: min(60s * 2^(attempt-1), 480s) + jitter
  — jitter: uniform random in [0, delay * 0.2]
  — attempt is 1-indexed (attempt=1 is the initial; retry 1 = attempt=2)

MaxAttempts = 4  (constant)
```

No external dependencies — pure computation.

## Tests

`internal/processor/retry/backoff_test.go`:

- `TestDelay_attempt2` — base = 60s; result in [60s, 72s]
- `TestDelay_attempt3` — base = 120s; result in [120s, 144s]
- `TestDelay_attempt4` — base = 240s; result in [240s, 288s]
- `TestDelay_cap` — attempt 10 → capped at 480s + jitter (≤ 576s)
- `TestDelay_jitter_nonNegative` — 1000 calls → all results >= base delay
- `TestDelay_jitter_bounded` — 1000 calls → all results <= base delay * 1.2
