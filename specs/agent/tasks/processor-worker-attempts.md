# processor-worker-attempts

**Specs:** `system/TODOS.md` § Processor Database
**Depends on:** `processor-db-package`, `remove-processing-status`
**Status:** pending

## Context

After this task the worker writes `delivery_attempts` directly to the processor's own DB
after each actual delivery attempt. The status stream publish is retained — the API still
reads it to update notification status on the `notifications` table.

`publishStatus` is called four times in the worker. Only three of those represent actual
delivery attempts (after `webhookClient.Send()` returns). The initial `processing` publish
before the send is removed by `remove-processing-status` — this task assumes that is
already done.

Delivery attempt write points:

| Branch | `delivery_attempts.status` |
|---|---|
| Success | `delivered` |
| Retryable failure | `failed` |
| Terminal failure | `failed` |

## What to build

### `processor/internal/worker/worker.go`

Add a port interface (keeps worker decoupled from the db package):
```go
type DeliveryAttemptWriter interface {
    Create(ctx context.Context, a *model.DeliveryAttempt) error
}
```

Add `attempts DeliveryAttemptWriter` to the `Worker` struct and `NewWorker` parameters.

In the `switch` block after `webhookClient.Send()`, call `w.attempts.Create` in each
delivery branch before the `publishStatus` call:

```go
attempt := &model.DeliveryAttempt{
    ID:            <new uuid v7>,
    NotificationID: <parsed uuid from evt.NotificationID>,
    AttemptNumber: evt.AttemptNumber,
    Status:        <"delivered" or "failed">,
    LatencyMS:     &dr.LatencyMS,
    AttemptedAt:   time.Now().UTC(),
}
// set HTTPStatusCode from dr.StatusCode if non-zero
// set ErrorMessage from dr.ErrorMessage if non-empty
if err := w.attempts.Create(ctx, attempt); err != nil {
    slog.ErrorContext(ctx, "write delivery attempt failed", "id", evt.NotificationID, "error", err)
    // non-fatal: log and continue — publishStatus must still fire
}
```

### `processor/cmd/main.go`

Wire `processordb.NewDeliveryAttemptRepository(bundb)` into `worker.NewWorker(...)`.

## Tests

`processor/internal/worker/worker_test.go` — inject a `mockAttemptWriter`:
- Success path: `Create` called once with `Status = "delivered"` and correct `AttemptNumber`
- Retryable failure path: `Create` called once with `Status = "failed"`
- Terminal failure path: `Create` called once with `Status = "failed"`
- `Create` returns error: worker does not abort — `publishStatus` still fires

## Verification

- `go test ./processor/internal/worker/...` passes
- Send a notification; after delivery query:
  ```sql
  SELECT * FROM processor.delivery_attempts;
  ```
  One row per attempt with correct `status`, `latency_ms`, and `http_status_code`
