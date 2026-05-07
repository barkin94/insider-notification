# stream-adapter

**Specs:** `system/QUEUE_DESIGN.md`, `system/MESSAGE_CONTRACT.md`
**Verification:** `api-service/VERIFICATION.md` § Stream Producer, `processor-service/VERIFICATION.md` § Stream Consumer & Worker Pool
**Status:** pending

## What to build

### `internal/shared/stream/message.go`
```
PriorityMessage struct:
  NotificationID string
  DeliverAfter   string  ← RFC3339 or empty

StatusMessage struct:
  NotificationID    string
  Status            string
  AttemptNumber     int
  HTTPStatusCode    int
  ErrorMessage      string
  ProviderMessageID string
  LatencyMS         int
  UpdatedAt         string
```

### `internal/shared/stream/producer.go`
```
Producer interface:
  Publish(ctx, priority string, msg PriorityMessage) error
  PublishStatus(ctx, msg StatusMessage) error

redisProducer struct{ client *redis.Client }
  — Publish → XADD notify:stream:{priority}
  — PublishStatus → XADD notify:stream:status
```

### `internal/shared/stream/consumer.go`
```
Consumer interface:
  ReadPriority(ctx) (*PriorityMessage, msgID string, err error)
  ReadStatus(ctx) (*StatusMessage, msgID string, err error)
  Ack(ctx, stream, msgID string) error
  ReclaimStale(ctx, stream string, minIdle time.Duration) error

redisConsumer struct{ client *redis.Client; groupName, consumerName string }
  — ReadPriority: sweep high → normal → low (non-blocking), then block on high (1s timeout)
  — Consumer group created on NewConsumer(); BUSYGROUP error swallowed
  — ReclaimStale: XAUTOCLAIM with minIdle threshold
```

## Tests

testcontainers-go with real Redis:

- `TestProducer_Publish_routesToCorrectStream` — high/normal/low messages land in correct streams
- `TestProducer_PublishStatus` — message lands in notify:stream:status
- `TestConsumer_ReadPriority_order` — high consumed before normal before low when all present
- `TestConsumer_Ack` — message removed from PEL after ack
- `TestConsumer_ReclaimStale` — unacked message reclaimed after minIdle
- `TestConsumer_groupCreatedIfAbsent` — NewConsumer on fresh Redis creates group without error
