# RETRY POLICY — Notification System

## Design Decision
Failed deliveries are retried using **exponential backoff with jitter**, capped at **4 total attempts**
(1 initial + 3 retries). Retries are re-enqueued into the same priority queue as the original
notification, with a `deliver_after` timestamp that the worker respects.

---

## Attempt Limits

| Setting | Value |
|---------|-------|
| Max total attempts | 4 (applies to all channels) |
| Initial attempt | counts as attempt #1 |
| Max retries after initial failure | 3 |

---

## Backoff Formula

```
base_delay   = 60 seconds
max_delay    = 480 seconds (8 minutes)
jitter_range = [0, delay * 0.2]   ← uniform random

delay(attempt) = min(base_delay * 2^(attempt - 1), max_delay) + random(jitter_range)
```

**Computed delays per attempt:**

| Attempt # | Base Delay | + Jitter (max) | Approx Window |
|-----------|-----------|----------------|---------------|
| 1 (initial) | — | — | immediate |
| 2 (retry 1) | 60s | +12s | ~1–1.2 min |
| 3 (retry 2) | 120s | +24s | ~2–2.4 min |
| 4 (retry 3) | 240s | +48s | ~4–4.8 min |

Total elapsed before final failure: ~7–8.4 minutes.

---

## Retry Eligibility

A failed delivery attempt is eligible for retry if ALL of the following are true:
- `notification.attempts < notification.max_attempts` (i.e. < 4)
- `notification.status` is `processing` (not `cancelled`)
- The failure is **retryable** (see non-retryable conditions below)

---

## Non-Retryable Conditions

These failures immediately move the notification to `failed` status with no further retries:

| Condition | Reason |
|-----------|--------|
| Provider returns HTTP 400 | Bad request — retrying won't help |
| Provider returns HTTP 401 / 403 | Auth error — configuration issue |
| Content validation failure | Content won't change between retries |
| Notification was cancelled between enqueue and dispatch | Status check before dispatch |

**Retryable conditions (will retry):**
- Provider returns HTTP 5xx
- Provider returns HTTP 429 (rate limited by provider — note: distinct from our own rate limiter)
- Network timeout or connection error
- Provider returns unexpected non-202 status

---

## Retry Worker Implementation

Workers are driven by `XREADGROUP` polling (see QUEUE_DESIGN.md), not by polling PostgreSQL.

Each stream message carries a `deliver_after` timestamp. If the message is not yet due, the worker re-enqueues it immediately and moves on — no retry budget is consumed.

Once a message is due, the worker acquires a Redis lock on the notification ID, then atomically transitions the notification from `pending` to `processing` in PostgreSQL while incrementing `attempts`. If the row was already grabbed by another worker, the message is ACKed and skipped.

Before dispatching, the worker checks the channel rate limiter. If exhausted, the notification is put back to `pending` with no backoff and no attempt counted.

On **success** (provider 202): status → `delivered`. A status event is published to `notify:stream:status` for the delivery audit record.

On **retryable failure** with attempts remaining: backoff delay is computed, `deliver_after` is set, status returns to `pending`, and the notification is re-enqueued into the same priority stream.

On **terminal failure** (non-retryable error, or attempts exhausted): status → `failed`. A final status event is published.

---

## Dead Letter Behavior

There is no separate dead-letter queue. Notifications that exhaust all attempts are marked
`status = 'failed'` in PostgreSQL with full `delivery_attempts` history. This provides:

- Full audit trail of all attempts, HTTP status codes, error messages, and latency
- Queryable via `GET /notifications?status=failed`
- Observable via metrics endpoint (`metrics.delivery.{channel}.failed` counter)

Manual reprocessing of failed notifications is **out of scope** for this implementation.

---

## Rate Limiter Interaction

The rate limiter check happens **before** the delivery attempt inside the worker loop.
If the token bucket for a channel is exhausted:
- The worker **does not** count this as a failed attempt
- The notification is re-enqueued immediately (no backoff delay applied)
- The worker sleeps for `1000ms / capacity` (10ms for 100 msg/s) before next dispatch
- This is transparent to the retry counter — only actual provider failures consume retry budget

---

## Metrics Emitted on Retry Events

| Event | Metric updated |
|-------|---------------|
| Delivery success | `metrics:sent:{channel}` +1, latency recorded |
| Delivery failure (will retry) | `metrics:failed:{channel}` +1 (temporary) |
| Notification moves to `failed` | permanent failed count in DB, visible via `/metrics` |
