# stream-adapter

**Specs:** `system/QUEUE_DESIGN.md`, `system/MESSAGE_CONTRACT.md`
**Verification:** `api-service/VERIFICATION.md` § Stream Producer, `processor-service/VERIFICATION.md` § Stream Consumer & Worker Pool
**Status:** complete

## What to build

### `internal/shared/stream/events.go`
```
NotificationCreatedEvent struct:
  NotificationID string
  Channel        string
  Recipient      string
  Content        string
  Priority       string
  AttemptNumber  int
  MaxAttempts    int
  DeliverAfter   string  ← RFC3339 or empty
  Metadata       string  ← JSON string, "{}" if absent

NotificationDeliveryResultEvent struct:
  NotificationID    string
  Status            string
  AttemptNumber     int
  HTTPStatusCode    int
  ErrorMessage      string
  ProviderMessageID string
  LatencyMS         int
  UpdatedAt         string  ← RFC3339

Trace context (traceparent, tracestate) travels in watermill message.Metadata,
not in the event payload. Injected/extracted by OTel middleware (observability task).
```

### `internal/shared/stream/topics.go`
```
TopicHigh, TopicNormal, TopicLow, TopicStatus  ← shared topic name constants
```

### `internal/shared/stream/publisher.go`
```
Publisher struct{ pub message.Publisher }
  Publish(ctx, topic string, payload any) error
    — JSON-encodes payload, publishes to topic via Watermill
    — callers choose topic and payload type; publisher has no business knowledge
```

### `internal/shared/stream/subscriber.go`
```
Subscribe[T any](ctx, sub message.Subscriber, topic string) (<-chan Result[T], error)
  — generic helper: subscribes to topic, decodes JSON payload into T
  — Nacks and forwards error on decode failure

Result[T] struct{ Event T; Msg *message.Message; Err error }
  — caller calls Msg.Ack() after processing or Msg.Nack() to requeue
```

## Tests

- `TestPublishNotificationCreated_routesToCorrectTopic` — high/normal/low events land in correct topics
- `TestPublishDeliveryResult` — event lands in TopicStatus and round-trips correctly

## Future work

See `priority-router` task for priority-ordered fan-in across the three priority topics.
OTel trace context injection/extraction via Watermill middleware: see `observability` task.
