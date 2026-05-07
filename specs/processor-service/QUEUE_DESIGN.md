# QUEUE DESIGN — Notification System

## Design Decision

Four named streams handle all inter-service messaging. Three carry notifications from API to
Processor in priority order; one carries delivery outcomes back from Processor to API.
Workers consume using consumer group semantics: each message is delivered to exactly one
worker, and unacknowledged messages are automatically reclaimable after a timeout.

---

## Streams

| Stream | Direction | Purpose |
|--------|-----------|---------|
| `notify:stream:high` | API → Processor | High-priority notifications |
| `notify:stream:normal` | API → Processor | Normal-priority notifications |
| `notify:stream:low` | API → Processor | Low-priority notifications |
| `notify:stream:status` | Processor → API | Delivery outcome events |

Payloads are intentionally minimal — full notification data is fetched from PostgreSQL after
a message is consumed.

**Priority stream message fields:**

| Field | Type | Description |
|-------|------|-------------|
| `notification_id` | UUID | Identifies the notification row in PostgreSQL |
| `deliver_after` | RFC3339 or empty | If set, worker defers processing until this time |

**Status stream message fields:** see `MESSAGE_CONTRACT.md`.

---

## Consumer Groups

| Group | Reads from | Used by |
|-------|-----------|---------|
| `notify:cg:processor` | priority streams | Processor workers |
| `notify:cg:api` | `notify:stream:status` | API status consumer |

Both groups are created on startup if they do not already exist.

---

## Priority Ordering

Workers sweep streams in priority order before blocking:

```
poll for next message:
          │
    high stream non-empty? ──yes──► return message
          │ no
    normal stream non-empty? ──yes──► return message
          │ no
    low stream non-empty? ──yes──► return message
          │ no
    block on high stream (1s timeout)
          │
    message arrived? ──yes──► return message
          │ no
    return nil
```

The non-blocking sweep prevents the worker from stalling on `high` while `normal` or `low`
messages accumulate.

---

## Worker Pool

10 workers run concurrently (configurable via `WORKER_CONCURRENCY`). Each worker loops:

1. Poll for next message using priority ordering above
2. Acquire a processing lock on the notification ID (TTL 60s) — skip if already held
3. Fetch notification from PostgreSQL; skip if status is not `pending` or `deliver_after` is in the future
4. Transition status to `processing` atomically — skip if another worker already did
5. Execute rate limit → delivery → retry logic (see `RETRY_POLICY.md`)
6. Acknowledge the message

---

## Enqueue

The API publishes a message to `notify:stream:{priority}` after creating a notification
(status = `pending`). The Processor re-publishes to the same stream when scheduling a retry,
setting `deliver_after` to the computed backoff time.

---

## Acknowledge

A message is acknowledged (removed from the pending entry list) after any terminal outcome:
successful delivery, terminal failure, or a skip condition (wrong status, lock miss).
Rate-limited notifications are re-enqueued rather than acknowledged in place.

---

## Status Events

After each delivery attempt the Processor publishes an event to `notify:stream:status`.
The API status consumer reads each event, writes a `delivery_attempts` row to PostgreSQL,
updates `notifications.status`, then acknowledges the message.

Full event shape: see `MESSAGE_CONTRACT.md`.

---

## Crash Recovery

The broker maintains a pending entry list (PEL) of messages delivered to a consumer but not
yet acknowledged. On Processor startup, any message idle in the PEL for more than 2 minutes
is reclaimed and re-processed. Because each worker re-checks `notification.status` in
PostgreSQL before acting, re-processing an already-delivered message is safe.

---

## Queue Depth

Queue depth is read from each stream and exposed as an OTel gauge
(`notification.queue.depth` labelled by priority), scraped by Prometheus.

---

## Starvation

Under sustained high-priority load, `normal` and `low` streams will starve. This is
intentional — OTPs and security alerts take precedence over marketing messages.
Aging-based promotion is deferred (see `TODOS.md`).
