# Processor

Consumes notification events from prioritised message topics and delivers them via a weighted priority router, a four-gate delivery pipeline, and an exponential-backoff retry mechanism.

## PriorityRouter — `internal/delivery/priorityrouter.go`

Schedules work across multiple channels using **weighted round-robin**. Higher-weight sources get proportionally more slots in the rotation schedule.

### Construction

Given weights `high=3, normal=2, low=1`, the router pre-expands them into a slots array:

```text
sources: [ high_ch   normal_ch   low_ch ]
           idx 0     idx 1       idx 2

slots:   [ 0, 0, 0,  1, 1,  2 ]
           H  H  H   N  N   L
```

Each `Next()` call picks the next slot via `cursor % len(slots)` and advances the cursor.

### Decision tree per `Next()` call

```text
Next() called
│
├─ Phase 1: scheduled channel has a message? (non-blocking)
│   yes → return it                     weights respected, fast path
│   no  ↓
│
├─ Phase 2: any channel has a message? (non-blocking scan, high → normal → low)
│   yes → return highest-priority one available
│   no  ↓
│
└─ Phase 3: block on high_ch with 1s idle timeout
    woke by message → return (value, true)
    timeout (1s)    → return (zero, false)  → WorkerPool retries from Phase 1
    ctx cancelled   → return (zero, false)  → WorkerPool exits
```

**Phase 1** enforces weighting — it only fires when the rotation lands on that source's slot.

**Phase 2** prevents wasted turns — if the scheduled channel is empty but another has work, it is taken regardless of whose slot it was.

**Phase 3** avoids busy-waiting when all channels are idle. Parking on `sources[0]` biases wake-ups toward high-priority messages. Normal/low messages that arrive while parked are picked up within 1s when the timeout fires and Phase 2 runs again.

## Delivery pipeline — `internal/delivery/pipeline.go`

Each message pulled from the router runs through four gates in sequence:

```text
NotificationReadyEvent
│
├─ 1. Lock          TryLock(notificationID)
│                   miss (already being processed) → Ack, skip
│                   error                          → Nack
│
├─ 2. Rate limit    IsAllowed(channel)
│                   allowed  → continue
│                   limited  → save payload, write delay to ZSET, Ack
│                   error    → Nack
│
├─ 3. Send          HTTP POST to notification provider
│                   202 Accepted        → success
│                   400 / 401 / 403     → non-retryable failure
│                   anything else / err → retryable failure
│
└─ 4. Record outcome
        success           → publish delivered status, clear attempt state
        retryable failure → save payload, create retry record with retry_after
        terminal failure  → publish failed status, clear attempt state
```

The pipeline Acks the message at the end of a successful gate sequence. It Nacks only when state could not be persisted (so the message is redelivered by the broker).

## Published events

Terminal outcomes (success or exhausted retries) are published to the status topic as `NotificationDeliveryResultEvent`:

| Field                 | Notes                              |
|-----------------------|------------------------------------|
| `notification_id`     |                                    |
| `status`              | `delivered` or `failed`            |
| `attempt_number`      | which attempt produced this result |
| `http_status_code`    |                                    |
| `provider_message_id` | set on success                     |
| `error_message`       | set on failure                     |
| `latency_ms`          |                                    |

## Retry mechanism

There are two independent retry paths.

### Delivery failure — exponential backoff

When a send fails with a retryable error and the attempt limit has not been reached, the pipeline writes a `DeliveryAttempt` record with a `retry_after` timestamp.

Backoff formula (1-indexed attempt number):

```text
delay = min(60s × 2^(attempt−1), 480s) + uniform jitter in [0, delay × 0.2]
```

| Attempt       | Base delay | Max with jitter |
|---------------|------------|-----------------|
| 2 (1st retry) | 60s        | 72s             |
| 3             | 120s       | 144s            |
| 4             | 240s       | 288s            |

Default max attempts is **4** (3 retries). A per-notification `max_attempts` override is accepted at creation time.

### Rate limit — delay queue

When the channel's token bucket is exhausted, the event is not retried immediately. Instead the payload is saved and a delay is written to a ZSET with `retry_after = now + retryAfter` (minimum 1s). The RetryDispatcher picks it up once the window expires.

### RetryDispatcher — `internal/transport/messaging/retrydispatcher.go`

A background ticker (default: every 1s) that polls for due attempts and re-enqueues them:

```text
every interval:
  GetDue(now, batch=100)
  → for each due attempt:
      republish NotificationReadyEvent to the original priority topic
      RemoveDue(notificationID)
```

Re-enqueuing to the original priority topic means retried messages go through the same weighted router as new messages — high-priority retries are not penalised.
