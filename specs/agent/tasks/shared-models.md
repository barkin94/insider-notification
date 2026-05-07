# shared-models

**Specs:** `api-service/DATA_MODEL.md`
**Verification:** `api-service/VERIFICATION.md` § Data Layer
**Status:** complete

## What to build

### `internal/shared/model/enums.go`

String constants for all enum values:

```
Channel:  ChannelSMS = "sms", ChannelEmail = "email", ChannelPush = "push"
Priority: PriorityHigh = "high", PriorityNormal = "normal", PriorityLow = "low"
Status:   StatusPending, StatusProcessing, StatusDelivered, StatusFailed, StatusCancelled
```

### `internal/shared/model/notification.go`

```
Notification struct — all columns from DATA_MODEL.md notifications table
  id, batch_id (nullable), recipient, channel, content, priority, status,
  idempotency_key (nullable), deliver_after (nullable), attempts, max_attempts,
  metadata (nullable jsonb), created_at, updated_at
  Tags: db:"..." for pgx scanning

DeliveryAttempt struct — all columns from DATA_MODEL.md delivery_attempts table
  id, notification_id, attempt_number, status, http_status_code (nullable),
  provider_response (nullable jsonb), error_message (nullable), latency_ms (nullable),
  attempted_at
  Tags: db:"..."

IdempotencyKey struct — all columns from DATA_MODEL.md idempotency_keys table
  key, notification_id, key_type, expires_at, created_at
  Tags: db:"..."
```

## Tests

`internal/shared/model/enums_test.go`

- `TestChannelValues` — assert string values match spec exactly
- `TestPriorityValues` — assert string values match spec exactly
- `TestStatusValues` — assert string values match spec exactly
