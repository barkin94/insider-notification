# scheduled-worker

**Specs:** `system/TODOS.md` § Scheduled Notifications
**Depends on:** `scheduled-api`
**Verification:** `processor-service/VERIFICATION.md`
**Status:** pending

## Context

The worker receives all notifications as `pending` events (API always publishes immediately).
When `deliver_after` is in the future on attempt 1, the worker ACKs and drops the event —
the notification stays `pending` in the DB. A scheduler goroutine polls the DB every 5
seconds for due notifications and publishes them to the priority queue.

Retry re-enqueue (attempt > 1) is unchanged — retries bounce in the Redis stream until
their backoff `deliver_after` passes. See `retry-requeue` task for future improvement.

The per-notification Redis lock prevents duplicate delivery if two scheduler ticks overlap.

## What to build

### `processor/internal/worker/worker.go`

Change the deliver_after branch to ACK-and-drop only on the first attempt:

```go
if evt.DeliverAfter != "" {
    if t, err := time.Parse(time.RFC3339, evt.DeliverAfter); err == nil && time.Now().Before(t) {
        if evt.AttemptNumber <= 1 {
            // initial scheduled delivery — DB-polling scheduler handles it
            msg.Ack()
            return
        }
        // retry backoff — bounce in stream until due
        if err := w.pub.Publish(ctx, topicByPriority[evt.Priority], evt); err != nil {
            slog.ErrorContext(ctx, "re-enqueue retry failed", "id", evt.NotificationID, "error", err)
            msg.Nack()
            return
        }
        msg.Ack()
        return
    }
}
```

### `processor/internal/scheduler/scheduler.go`

```go
type Scheduler struct {
    db        *bun.DB
    publisher StreamPublisher
    interval  time.Duration
}

func New(db *bun.DB, publisher StreamPublisher) *Scheduler {
    return &Scheduler{db: db, publisher: publisher, interval: 5 * time.Second}
}

func (s *Scheduler) Run(ctx context.Context) {
    ticker := time.NewTicker(s.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.tick(ctx)
        }
    }
}
```

`tick` logic:

1. Query: `SELECT * FROM notifications WHERE deliver_after IS NOT NULL AND deliver_after <= NOW() AND status = 'pending' ORDER BY deliver_after ASC LIMIT 500`
2. For each row: build `NotificationCreatedEvent` (AttemptNumber = 1, DeliverAfter = "") and publish to `topicByPriority[n.Priority]`
3. Log publish errors but continue processing remaining rows

`StreamPublisher` is the same interface defined in `processor/internal/worker/worker.go`.
Move it to a shared internal package (e.g. `processor/internal/pub`) to avoid importing
worker from scheduler, or re-declare it in the scheduler package (interfaces are structural).

### `processor/cmd/main.go`

Start the scheduler as a goroutine tied to the same context:

```go
sched := scheduler.New(bundb, pub)
go sched.Run(ctx)
```

## Tests

### `processor/internal/worker/worker_test.go`

Update deliver_after tests to match the new behaviour:

- `TestWorker_deliverAfter_future_attempt1` — `deliver_after` = now+1h, attempt 1:
  no publish, Ack called, webhook not called.
- `TestWorker_deliverAfter_future_retryAttempt` — `deliver_after` = now+1h, attempt 2:
  re-enqueue publish called, Ack called, webhook not called.
- `TestWorker_deliverAfter_past` — `deliver_after` = now-1s: falls through to delivery.
- `TestWorker_deliverAfter_empty` — empty `DeliverAfter`: falls through to delivery.

### `processor/internal/scheduler/scheduler_test.go` — testcontainers-go (real postgres + redis)

- `TestTick_enqueuesWhenDue` — `pending` notification with `deliver_after = now-1s`;
  run one `tick`; assert event published to the correct priority stream.
- `TestTick_skipsNotYetDue` — `deliver_after = now+1h`; run one `tick`; assert no publish.
- `TestTick_skipsNilDeliverAfter` — `deliver_after IS NULL`; run one `tick`; assert no publish.

## Verification

- `go test ./processor/internal/worker/... ./processor/internal/scheduler/...` passes
- `POST /notifications` with `deliver_after` = now+30s; wait 35s; notification reaches `delivered`
- `POST /notifications` with `deliver_after` = now+30s; cancel immediately; stays `cancelled`
