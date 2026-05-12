# scheduled-worker

**Specs:** `system/TODOS.md` § Scheduled Notifications
**Depends on:** `scheduled-api`
**Verification:** `processor-service/VERIFICATION.md`
**Status:** pending

## Context

The scheduler goroutine lives in the **processor** service — delivery timing is a processor
concern. The processor's `DATABASE_URL` already points to the `notifications` database with
`search_path=processor,public`, so it can query the `notifications` table (public schema)
directly via the same connection used for `delivery_attempts`.

Every 5 seconds the scheduler finds `status = scheduled AND deliver_after <= NOW()`,
transitions each to `pending`, and publishes to the priority queue. From there the worker
handles them identically to any other `pending` notification.

`idx_notifications_deliver_after_status` already covers the query — no new index needed.

## What to build

### `processor/internal/scheduler/scheduler.go`

```go
type Scheduler struct {
    db        *bun.DB
    publisher StreamPublisher  // same interface used by Worker
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
1. Query: `SELECT * FROM notifications WHERE status = 'scheduled' AND deliver_after <= NOW() ORDER BY deliver_after ASC LIMIT 500`
2. For each row, in a loop:
   - `UPDATE notifications SET status = 'pending', updated_at = NOW() WHERE id = ? AND status = 'scheduled'` — skip silently if 0 rows updated (race with a cancel)
   - Build `NotificationCreatedEvent` from the row and publish to `topicByPriority[n.Priority]`
   - Log publish errors but continue processing the remaining rows

`StreamPublisher` is the same interface already defined in `processor/internal/worker/worker.go`
— reference it from there rather than redefining.

### `processor/cmd/main.go`

Start the scheduler as a goroutine alongside the worker pool, tied to the same context:
```go
sched := scheduler.New(bundb, pub)
go sched.Run(ctx)
```

## Spec files to update

### `specs/system/ARCHITECTURE.md`
- Add Scheduler as a component inside the Processor service box in the diagram
- Add ADR: scheduler in processor (delivery timing is a processor concern; processor DB
  connection already covers the notifications table via search_path)
- Add `processor/internal/scheduler/` to the project layout
- Add ADR for polling approach: no cron dependency; 5s granularity; up to 5s delivery
  delay is an accepted tradeoff

### `specs/processor-service/QUEUE_DESIGN.md`
- Add scheduler as a second producer for the priority streams alongside the API

### `specs/processor-service/VERIFICATION.md`
- Add: `scheduled` notification enqueued and delivered within ~5s of `deliver_after`
- Add: cancelling a `scheduled` notification before `deliver_after` prevents delivery

## Tests

`processor/internal/scheduler/scheduler_test.go` using testcontainers-go (real postgres + redis):

- `TestTick_enqueuesWhenDue` — insert a `scheduled` notification with `deliver_after = now-1s`;
  run one `tick`; assert status transitioned to `pending` and event published to stream
- `TestTick_skipsNotYetDue` — `deliver_after = now+1h`; run one `tick`; assert no change
- `TestTick_skipsCancelledRace` — notification cancelled between SELECT and UPDATE;
  UPDATE affects 0 rows; assert no publish and no error

## Verification

- `go test ./processor/internal/scheduler/...` passes
- `POST /notifications` with `deliver_after` = now+65s; wait ~70s; notification reaches `delivered`
- `POST /notifications` with `deliver_after` = now+65s; immediately cancel; notification
  stays `cancelled` after `deliver_after` passes
