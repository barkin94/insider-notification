# status-consumer

**Specs:** `system/MESSAGE_CONTRACT.md`, `system/QUEUE_DESIGN.md`
**Verification:** `api-service/VERIFICATION.md` § Status Event Consumer
**Status:** pending

## What to build

### `api/internal/consumer/status.go`
```
StatusConsumer struct:
  consumer  stream.Consumer
  notifRepo db.NotificationRepository
  attemptRepo db.DeliveryAttemptRepository
  logger    *slog.Logger

NewStatusConsumer(...) *StatusConsumer

Run(ctx context.Context)
  — loop until ctx cancelled:
    1. consumer.ReadStatus(ctx) → StatusMessage, msgID
    2. Parse fields
    3. attemptRepo.Create(ctx, &DeliveryAttempt{...})  ← ON CONFLICT DO NOTHING
    4. notifRepo.Transition or direct status update based on msg.Status
    5. consumer.Ack(ctx, "notify:stream:status", msgID)
    6. Log event
```

## Tests

testcontainers-go (real Redis + PostgreSQL):

- `TestStatusConsumer_delivered` — status=delivered message → notification.status=delivered + delivery_attempt row inserted
- `TestStatusConsumer_failed` — status=failed → notification.status=failed
- `TestStatusConsumer_processing` — status=processing → notification.status=processing
- `TestStatusConsumer_idempotent` — duplicate message (same notification_id + attempt_number) → no error, no duplicate row
