# QUEUE DESIGN — Notification System

## Design Decision
Three **separate Redis Lists** — one per priority level. Workers use **blocking pop with fallback**:
always attempt to pop from `high` first, then `normal`, then `low`. This guarantees high-priority
messages are processed before normal/low under any load condition.

---

## Queue Names

```
notify:queue:high    ← BRPOPLPUSH source for high priority
notify:queue:normal  ← BRPOPLPUSH source for normal priority
notify:queue:low     ← BRPOPLPUSH source for low priority
```

Queue values are notification UUIDs (strings). Full notification data is fetched from MongoDB
by the worker after dequeue. This keeps queue payloads minimal and avoids stale data in the queue.

---

## Worker Poll Algorithm

```
FUNCTION poll_next_notification():
  // Try each queue in priority order, non-blocking
  FOR queue IN [high, normal, low]:
    id = RPOP notify:queue:{queue}
    IF id != nil:
      RETURN id

  // All queues empty — block on high queue with timeout
  id = BRPOP notify:queue:high TIMEOUT 1s
  IF id != nil:
    RETURN id

  RETURN nil   // nothing in any queue after timeout
```

**Rationale for non-blocking first pass:** Avoids BRPOP blocking on `high` while `normal`/`low`
items accumulate. The non-blocking sweep ensures any available item is picked up regardless of
which queue it's in, while still preferring higher priority.

---

## Worker Pool

```
CONCURRENCY = 10 workers (configurable via WORKER_CONCURRENCY env var)

Each worker runs in its own goroutine:
  LOOP:
    id = poll_next_notification()
    IF id == nil: continue

    acquire Redis lock notify:lock:{id} TTL=60s
    IF lock not acquired: continue   ← another worker grabbed it

    fetch notification from MongoDB notifications collection WHERE _id = {id}
    IF notification.status != 'pending': release lock, continue
    IF notification.deliver_after > NOW(): re-enqueue, release lock, continue

    process notification (rate limit → deliver → retry logic)
    release lock
```

---

## Enqueue Operation

```
FUNCTION enqueue(notification_id UUID, priority priority_type):
  key = notify:queue:{priority}
  LPUSH key notification_id         ← push to head (LIFO within same priority)
  INCR metrics:queue_depth:{priority}
```

**Called by:**
- API handler after successful notification creation
- Retry logic after computing `deliver_after` delay

---

## Dequeue Operation

```
FUNCTION dequeue(notification_id UUID, priority priority_type):
  DECR metrics:queue_depth:{priority}
  // actual RPOP/BRPOP already done in poll_next_notification
```

---

## Starvation Consideration

**Known tradeoff:** Under sustained high-priority load, `normal` and `low` queues will starve.

**Accepted:** This is intentional behavior for a notification system. High-priority notifications
(e.g. OTPs, security alerts) must be delivered first. Low-priority (e.g. marketing) can wait.

**Mitigation for future work (out of scope):** Implement aging — after a configurable threshold
(e.g. 5 minutes in queue), promote `low` → `normal` and `normal` → `high`.

---

## Queue Depth Tracking

Queue depth is tracked via Redis counters (`metrics:queue_depth:{priority}`) updated on every
enqueue and dequeue. These are exposed via the `GET /metrics` endpoint.

For accuracy, the counters are reconciled against `LLEN notify:queue:{priority}` on startup
to correct any drift from crashes.

---

## Persistence & Durability

Redis is configured with `appendonly yes` (AOF persistence). In the event of a Redis crash:
- Notifications already in the queue that haven't been popped are recovered from AOF
- Notifications that were popped but not yet processed (worker crash mid-flight) are recovered
  by a startup reconciliation job that re-enqueues any `processing` notifications older than 2 minutes

**Startup reconciliation:**
```go
// Find stuck notifications
cutoff := time.Now().UTC().Add(-2 * time.Minute)
filter := bson.M{
  "status":     "processing",
  "updated_at": bson.M{"$lt": cutoff},
}
update := bson.M{
  "$set": bson.M{
    "status":       "pending",
    "deliver_after": time.Now().UTC(),
    "updated_at":   time.Now().UTC(),
  },
}
// Use UpdateMany, then re-query to get IDs for re-enqueue
cursor, _ := col.Find(ctx, filter)
col.UpdateMany(ctx, filter, update)
// re-enqueue each recovered notification into its priority queue
```
