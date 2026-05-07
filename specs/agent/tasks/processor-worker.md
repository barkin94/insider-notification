# processor-worker

**Specs:** `system/QUEUE_DESIGN.md`, `processor-service/RETRY_POLICY.md`
**Verification:** `processor-service/VERIFICATION.md` § Stream Consumer & Worker Pool, § Worker End-to-End
**Status:** pending

## What to build

### `internal/processor/worker/worker.go`
```
Worker struct:
  consumer    stream.Consumer
  producer    stream.Producer
  notifRepo   db.NotificationRepository
  deliverer   delivery.Client
  limiter     ratelimit.Limiter
  redis       *redis.Client
  logger      *zap.Logger

Run(ctx context.Context)
  — loop until ctx cancelled: processNext(ctx)

processNext(ctx):
  1. consumer.ReadPriority(ctx) → msg, msgID
  2. If msg.DeliverAfter > now → producer.Publish (re-enqueue), consumer.Ack, return
  3. Acquire Redis lock notify:lock:{notification_id} (SET NX EX 60)
     → miss: consumer.Ack, return
  4. notifRepo.GetByID → if status != pending: consumer.Ack, return
  5. notifRepo.Transition(pending → processing) → error: consumer.Ack, return
  6. limiter.Allow(ctx, channel)
     → denied: notifRepo.Transition(processing → pending), re-enqueue, consumer.Ack, return
  7. delivery.Send(ctx, notification, correlationID) → Result
  8. notifRepo.IncrementAttempts
  9. If Result.Success:
       notifRepo.Transition(processing → delivered)
       producer.PublishStatus(delivered event)
  10. Else if Result.Retryable AND attempts < MaxAttempts:
       deliverAfter = time.Now().Add(retry.Delay(attempts))
       re-enqueue with deliver_after
       producer.PublishStatus(processing event)
  11. Else:
       notifRepo.Transition(processing → failed)
       producer.PublishStatus(failed event)
  12. consumer.Ack
  13. Release lock (DEL notify:lock:{id})
```

## Tests

`internal/processor/worker/worker_test.go` — inject mock consumer, producer, repo, deliverer, limiter:

- `TestWorker_delivered` — provider 202 → status=delivered, status event published
- `TestWorker_retryable_requeued` — provider 503, attempts=1 → re-enqueued with deliver_after, status event processing
- `TestWorker_exhausted_failed` — provider 503, attempts=4 → status=failed, no re-enqueue
- `TestWorker_nonRetryable_failed` — provider 400, attempts=1 → status=failed immediately
- `TestWorker_lockMiss` — lock already held → message acked, no DB write
- `TestWorker_deliverAfter_requeued` — deliver_after in future → re-enqueued, no delivery attempt
- `TestWorker_rateLimited_requeued` — limiter denied → re-enqueued, attempt counter unchanged
- `TestWorker_cancelled` — notification status=cancelled → skipped after lock
