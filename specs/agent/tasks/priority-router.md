# priority-router

**Specs:** `processor-service/QUEUE_DESIGN.md`
**Verification:** `processor-service/VERIFICATION.md` § Worker Pool
**Status:** pending

## What to build

### `internal/shared/stream/priority_router.go`
```
PriorityRouter struct{ high, normal, low <-chan Result[NotificationCreatedEvent] }

NewPriorityRouter(ctx, sub message.Subscriber) (*PriorityRouter, error)
  — subscribes to TopicHigh, TopicNormal, TopicLow
  — returns a PriorityRouter

Next(ctx) (*Result[NotificationCreatedEvent], error)
  — non-blocking check: high → normal → low
  — if all empty, blocks on high (1s timeout), then returns nil
```

Priority sweep logic:
```
1. select high (non-blocking default) → return if message
2. select normal (non-blocking default) → return if message
3. select low (non-blocking default) → return if message
4. select high with 1s timeout → return message or nil
```

## Tests

- `TestPriorityRouter_order` — messages in all three streams: high consumed before normal before low
- `TestPriorityRouter_returnsNil_whenEmpty` — no messages → Next returns nil after timeout
