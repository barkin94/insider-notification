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

Priority stream messages carry the full notification payload so the Processor requires no
PostgreSQL access. The API embeds all delivery-relevant fields at enqueue time.

**Priority stream message fields:** see `MESSAGE_CONTRACT.md`.

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

> **Superseded.** Strict priority sweep has been replaced by weighted round-robin.
> See [`PRIORITY_ROUTER.md`](./PRIORITY_ROUTER.md) for the authoritative algorithm,
> interface, and test specification.

---

## Worker Pool

10 workers run concurrently (configurable via `WORKER_CONCURRENCY`). Each worker loops:

1. Poll for next message via `PriorityRouter.Next()` (see `PRIORITY_ROUTER.md`)
2. If `deliver_after` is set and `now < deliver_after` — re-enqueue to same priority topic, ACK, and skip
3. Check cancellation store — if cancelled, ACK and skip
4. Acquire a processing lock on the notification ID (TTL 60s) — if already held, ACK and skip
5. Check rate limiter — if denied, re-enqueue to same priority topic, ACK, and skip
6. Execute delivery → retry logic (see `RETRY_POLICY.md`)
7. Publish `delivered` or `failed` status event, release lock, ACK the message

---

## Enqueue

The API publishes a message to `notify:stream:{priority}` after creating a notification
(status = `pending`), embedding the full notification payload.

On cancellation the API sets `SET cancelled:{id} 1 EX {ttl}` in Redis (TTL = 24 hours,
covers the maximum retry window) so in-flight workers can detect and skip the notification.

The API re-publishes to the same stream when scheduling a retry, incrementing `attempt_number`
and setting `deliver_after` to the computed backoff time.

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
is reclaimed and re-processed. Re-processing is safe because:
- A `cancelled:{id}` Redis key is checked before delivery
- The processing lock prevents two workers from delivering the same message concurrently
- The API's status consumer uses `ON CONFLICT DO NOTHING` on delivery_attempts rows

---

## Queue Depth

Queue depth is read from each stream and exposed as an OTel gauge
(`notification.queue.depth` labelled by priority), scraped by Prometheus.

