# MESSAGE CONTRACT — Notification System

Redis Streams message schemas for inter-service communication.
Messages are published via Watermill's Redis Streams adapter. Each message has:
- **Payload**: JSON-encoded event body
- **Metadata**: key-value string map carried alongside the payload (header equivalent).
  `traceparent` and `tracestate` travel here, injected/extracted by OTel middleware.

---

## API → Processor: Priority Streams (`NotificationCreatedEvent`)

Published by the Notification Management API on notification creation and retry re-enqueue.
Consumed by Notification Processor workers via consumer group `notify:cg:processor`.

**Topics:** `notify:stream:high`, `notify:stream:normal`, `notify:stream:low`

The full notification payload is embedded so the Processor requires no PostgreSQL access.
On retry re-enqueue the API increments `attempt_number` before publishing.

**Payload fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `notification_id` | UUID string | yes | Notification identifier |
| `channel` | string | yes | `sms` \| `email` \| `push` |
| `recipient` | string | yes | Destination address (phone, email, device token) |
| `content` | string | yes | Message body |
| `priority` | string | yes | `high` \| `normal` \| `low` |
| `attempt_number` | integer | yes | 1-indexed; 1 on first enqueue, incremented on each retry |
| `max_attempts` | integer | yes | Maximum allowed attempts |
| `deliver_after` | RFC3339 string | no | If set, worker defers until `now >= deliver_after`. Empty means deliver immediately. |
| `metadata` | JSON string | no | Arbitrary key-value pairs; `{}` if absent |

**Metadata fields (not in payload):**

| Key | Description |
|-----|-------------|
| `traceparent` | W3C trace context; injected by OTel middleware on publish |
| `tracestate` | W3C tracestate; empty if absent |

---

## Processor → API: Status Stream (`NotificationDeliveryResultEvent`)

Published by the Notification Processor after each delivery attempt.
Consumed by the API service's status consumer via consumer group `notify:cg:api`.

**Topic:** `notify:stream:status`

**Payload fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `notification_id` | UUID string | yes | Matches the notification row in PostgreSQL |
| `status` | string | yes | `processing` \| `delivered` \| `failed` |
| `attempt_number` | integer | yes | 1-indexed attempt counter |
| `http_status_code` | integer | no | Provider HTTP response code; 0 on network error |
| `error_message` | string | no | Human-readable failure reason; empty on success |
| `provider_message_id` | string | no | Provider-assigned message ID; populated on success |
| `latency_ms` | integer | yes | Time from dispatch to provider response in milliseconds |
| `updated_at` | RFC3339 string | yes | Timestamp of the attempt |

**Metadata fields (not in payload):**

| Key | Description |
|-----|-------------|
| `traceparent` | W3C trace context; propagated from the originating API request |
| `tracestate` | W3C tracestate; empty if absent |

---

## API Status Consumer Behaviour

On each `notify:stream:status` message the API service consumer:

1. Parses the JSON payload into `NotificationDeliveryResultEvent`
2. Inserts a row into `delivery_attempts` (idempotent via `ON CONFLICT DO NOTHING` on `notification_id + attempt_number`)
3. Updates `notifications.status` and `notifications.updated_at`
4. Calls `msg.Ack()` after DB writes complete
