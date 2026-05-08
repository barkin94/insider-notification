# notification-processing

**Specs:** `system/MESSAGE_CONTRACT.md`, `system/ARCHITECTURE.md`
**Status:** pending

## What to build

Implement `processOne` in `processor/internal/worker/worker.go` and its helpers.

### Delivery pipeline (in order)

1. **DeliverAfter** — if `evt.DeliverAfter` is set and still in the future, re-enqueue to the same priority topic (no attempt budget consumed) and Ack
2. **Cancellation check** — call `CancellationStore.IsCancelled`; if cancelled, Ack and skip
3. **Distributed lock** — call `Locker.TryLock`; on miss Ack and skip, on error Nack
4. **Rate limit** — call `ratelimit.Limiter.Allow`; if denied, re-enqueue and Ack
5. **Deliver** — call `delivery.Client.Send`; on transport error Nack
6. **Publish result** — publish `NotificationDeliveryResultEvent` to `TopicStatus`:
   - Success → status `delivered`
   - Retryable + attempts remaining → status `pending`, increment AttemptNumber, set DeliverAfter via `retry.Delay`, re-enqueue
   - Otherwise → status `failed`
7. Ack

### Helpers to restore

- `reEnqueue(ctx, evt)` — publish event to `topicByPriority[evt.Priority]`
- `publishStatus(ctx, evt, status, result)` — build and publish `NotificationDeliveryResultEvent`
- `mustParseUUID(s)` — parse UUID or return `uuid.Nil`

## Tests to write

Restore in `processor/internal/worker/worker_test.go` (unit, mock dependencies):

- `TestWorker_delivered` — success result → 1 status event with status=delivered
- `TestWorker_retryable_requeued` — retryable failure + attempts remaining → status event + re-enqueue with AttemptNumber+1 and DeliverAfter set
- `TestWorker_exhausted_failed` — retryable failure at max attempts → status=failed, no re-enqueue
- `TestWorker_nonRetryable_failed` — non-retryable failure → status=failed, no re-enqueue
- `TestWorker_lockMiss` — lock not acquired → no publish calls
- `TestWorker_deliverAfter_requeued` — future DeliverAfter → re-enqueue only, no status event
- `TestWorker_rateLimited_requeued` — rate limited → re-enqueue, same AttemptNumber, no status event
- `TestWorker_cancelled` — cancelled notification → no publish calls
