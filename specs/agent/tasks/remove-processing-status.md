# remove-processing-status

**Specs:** `api-service/DATA_MODEL.md`, `api-service/API_CONTRACT.md`, `system/MESSAGE_CONTRACT.md`, `processor-service/RETRY_POLICY.md`, `processor-service/QUEUE_DESIGN.md`, `api-service/VERIFICATION.md`, `processor-service/VERIFICATION.md`
**Status:** complete

## Context

`processing` is a transient status (milliseconds to seconds) with no actionable value for
clients. It is also semantically broken in the retryable-failure case, where the worker
publishes `Status: processing` with an error code and HTTP status — a contradiction.
The actual retry-eligibility check never reads the DB status; it uses the Redis
cancellation key. Removing `processing` makes the status machine honest:

```
pending → delivered
pending → failed
pending → cancelled
```

## What to remove

### `shared/model/enums.go`
- Delete `StatusProcessing = "processing"` constant

### `shared/model/enums_test.go`
- Remove all test cases referencing `StatusProcessing` / `"processing"`

### `processor/internal/worker/worker.go`
- Delete the `publishStatus` call before `webhookClient.Send()` (the `pending → processing`
  transition at the start of delivery)
- Delete the `publishStatus` call in the retryable-failure branch (the one that publishes
  `Status: model.StatusProcessing` with an error code after a failed attempt)
- The success and terminal-failure `publishStatus` calls are unchanged

### `api/internal/service/notification.go`
- In `Cancel`: the `Transition(from=StatusPending, to=StatusCancelled)` call is already
  correct. Remove any reference to `StatusProcessing` as a guard or comment.

### `api/internal/handler/notification.go` (and handler tests)
- Remove `processing` from any status filter allowlist or validation if present

### `api/internal/consumer/status.go`
- The consumer processes whatever events arrive; no code change needed here beyond ensuring
  no special-case branch exists for `processing`

## Spec files to update

### `specs/api-service/DATA_MODEL.md`
- Remove `processing` from the status enum values
- Remove the `[pending] → [processing]` arc from the status transition diagram
- Update transition table to show only `pending → delivered | failed | cancelled`

### `specs/api-service/API_CONTRACT.md`
- Remove `processing` from the `status` filter values on `GET /notifications`
- Remove `processing` from the 409 condition on `DELETE /notifications/:id/cancel`
  (cancel now returns 409 only for `delivered`, `failed`, `cancelled`)

### `specs/system/MESSAGE_CONTRACT.md`
- Remove `processing` from the `status` field enum on `NotificationDeliveryResultEvent`

### `specs/processor-service/RETRY_POLICY.md`
- Remove the "publish `processing` status event" step from the flowchart
- Fix the stale retry-eligibility condition: replace "`notification.status` is `processing`"
  with "notification is not cancelled (Redis cancellation key is absent)"
- Remove the retryable-failure branch line that publishes a status event

### `specs/processor-service/QUEUE_DESIGN.md`
- Remove step 6 ("Publish `processing` status event to `notify:stream:status`") from the
  worker loop description

### `specs/api-service/VERIFICATION.md`
- Remove checklist items that reference `processing` status events or transitions

### `specs/processor-service/VERIFICATION.md`
- Remove the checklist item: "Retryable failure … status event `processing` published"

## Tests

- `go test ./shared/model/...` — passes with `processing` removed from enum tests
- `go test ./processor/internal/worker/...` — passes; worker tests no longer expect
  a `processing` publish before delivery or on retryable failure
- `go test ./api/internal/...` — passes; consumer and handler tests updated accordingly

## Verification

- `grep -r "StatusProcessing\|\"processing\"" --include="*.go"` returns no matches outside
  of test fixtures that explicitly test the absence of the value
- `grep -r "processing" specs/` returns only incidental uses (e.g. "stream message processing"
  in OBSERVABILITY.md) — no status-value references
