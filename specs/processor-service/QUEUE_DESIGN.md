# QUEUE DESIGN — Notification System

## Design Decision
Three **separate Redis Streams** — one per priority level — plus a **status event stream** from
Processor back to API. Workers use **`XREADGROUP` with priority ordering**: always attempt the
high stream first, then normal, then low. Consumer group semantics provide at-least-once
delivery and crash recovery via the pending entry list (PEL).

---

## Stream Names

```
notify:stream:high      ← XREADGROUP source for high-priority notifications
notify:stream:normal    ← XREADGROUP source for normal-priority notifications
notify:stream:low       ← XREADGROUP source for low-priority notifications
notify:stream:status    ← Processor → API: delivery outcome events
```

Stream message values are key-value pairs. Payloads are intentionally minimal:
full notification data is fetched from PostgreSQL by the Processor after reading the stream.

**Priority stream message fields:**
```
notification_id   UUID string
deliver_after     RFC3339 timestamp, or empty string if immediate
```

**Status stream message fields:** (see MESSAGE_CONTRACT.md for full shape)
```
notification_id     UUID string
status              delivered | failed | processing
attempt_number      integer
http_status_code    integer (may be empty on network error)
error_message       string (may be empty)
provider_message_id string (may be empty)
latency_ms          integer
updated_at          RFC3339 timestamp
```

---

## Consumer Groups

```
notify:cg:processor   ← Processor workers reading from priority streams
notify:cg:api         ← API status consumer reading from notify:stream:status
```

Created on startup with `XGROUP CREATE ... MKSTREAM $` if they do not already exist.

---

## Worker Poll Algorithm (Processor)

```
FUNCTION poll_next():
  // Non-blocking sweep in priority order
  FOR stream IN [notify:stream:high, notify:stream:normal, notify:stream:low]:
    msgs = XREADGROUP GROUP notify:cg:processor {worker_id}
           COUNT 1 STREAMS {stream} >
    IF msgs != []:
      RETURN msgs[0]

  // All streams empty — block on high with 1s timeout
  msgs = XREADGROUP GROUP notify:cg:processor {worker_id}
         COUNT 1 BLOCK 1000 STREAMS notify:stream:high >
  RETURN msgs[0] or nil
```

**Rationale for non-blocking first sweep:** Prevents BRPOP-style blocking on `high` while
`normal`/`low` items accumulate. Any available item is picked up immediately, with priority
ordering preserved.

---

## Worker Pool (Processor)

```
CONCURRENCY = 10 workers (configurable via WORKER_CONCURRENCY env var)

Each worker runs in its own goroutine:
  LOOP:
    msg = poll_next()
    IF msg == nil: continue

    acquire Redis lock notify:lock:{notification_id} TTL=60s
    IF lock not acquired: XACK stream notify:cg:processor {msg_id}, continue

    fetch notification from PostgreSQL WHERE id = {notification_id}
    IF notification.status != 'pending': XACK, release lock, continue
    IF notification.deliver_after > NOW(): re-enqueue, XACK, release lock, continue

    SET status = 'processing' (via UPDATE WHERE status='pending' RETURNING)
    IF no rows updated: XACK, release lock, continue  ← another worker grabbed it

    process notification (rate limit → deliver → retry logic)
    XACK stream notify:cg:processor {msg_id}
    release lock
```

---

## Enqueue Operation (API → Priority Streams)

```
FUNCTION enqueue(notification_id UUID, priority string, deliver_after time.Time):
  stream = notify:stream:{priority}
  XADD stream * notification_id {uuid} deliver_after {rfc3339 or ""}
```

**Called by:**
- API handler after successful notification creation (status = `pending`)
- Retry logic after computing `deliver_after` delay (re-enqueues into same priority stream)

---

## Acknowledge Operation

```
XACK notify:stream:{priority} notify:cg:processor {msg_id}
```

Called after:
- Successful delivery
- Terminal failure (status = `failed`)
- Skip conditions (wrong status, lock miss)

---

## Status Event Publication (Processor → API)

After each delivery attempt, Processor publishes to `notify:stream:status`:

```
XADD notify:stream:status * \
  notification_id   {uuid} \
  status            {delivered|failed|processing} \
  attempt_number    {n} \
  http_status_code  {code} \
  error_message     {msg} \
  provider_message_id {id} \
  latency_ms        {n} \
  updated_at        {rfc3339}
```

The API status event consumer (`notify:cg:api`) reads this stream and:
1. Inserts a `delivery_attempts` row in PostgreSQL
2. Updates `notifications.status` accordingly
3. ACKs the message

---

## Crash Recovery

Redis Streams maintain a **pending entry list (PEL)**: messages delivered to a consumer but
not yet ACKed. On Processor startup:

```
FOR stream IN [notify:stream:high, notify:stream:normal, notify:stream:low]:
  XAUTOCLAIM notify:stream:{priority} notify:cg:processor {worker_id}
    MIN-IDLE-TIME 120000   ← reclaim messages idle > 2 minutes
    START 0-0
    COUNT 100
```

Reclaimed messages are re-processed from the beginning of the worker loop. Because the
worker re-checks `notification.status` in PostgreSQL before acting, re-processing an
already-delivered message is safe (idempotent status guard).

---

## Queue Depth Tracking

Queue depth is read via `XLEN notify:stream:{priority}` and exposed as an OTel gauge
(`notification.queue.depth`) scraped by Prometheus on each metrics collection interval.

---

## Starvation Consideration

**Known tradeoff:** Under sustained high-priority load, `normal` and `low` streams will starve.

**Accepted:** High-priority notifications (OTPs, security alerts) must be delivered first.
Low-priority (marketing) can wait.

**Future mitigation (out of scope):** Aging — after a configurable threshold, promote
`low` → `normal` and `normal` → `high`.
