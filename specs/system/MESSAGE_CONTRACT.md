# MESSAGE CONTRACT — Notification System

Redis Streams message schemas for inter-service communication.
All fields are key-value string pairs within a single Redis Stream entry.

---

## API → Processor: Priority Streams

Published by the Notification Management API on notification creation and retry re-enqueue.
Consumed by Notification Processor workers via consumer group `notify:cg:processor`.

**Streams:** `notify:stream:high`, `notify:stream:normal`, `notify:stream:low`

The full notification payload is embedded in each message so the Processor requires no
PostgreSQL access. On retry re-enqueue the API increments `attempt_number` before publishing.

**Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `notification_id` | UUID string | yes | Notification identifier |
| `channel` | string | yes | `sms` \| `email` \| `push` |
| `recipient` | string | yes | Destination address (phone, email, device token) |
| `content` | string | yes | Message body |
| `priority` | string | yes | `high` \| `normal` \| `low` |
| `attempt_number` | integer string | yes | 1-indexed; 1 on first enqueue, incremented on each retry |
| `max_attempts` | integer string | yes | Maximum allowed attempts |
| `deliver_after` | RFC3339 string | no | If set, worker defers until `now >= deliver_after`. Empty string means deliver immediately. |
| `metadata` | JSON string | no | Arbitrary key-value pairs; empty JSON object `{}` if absent |

**Example:**
```
XADD notify:stream:high *
  notification_id 550e8400-e29b-41d4-a716-446655440000
  channel         sms
  recipient       +15551234567
  content         "Your OTP is 482910"
  priority        high
  attempt_number  1
  max_attempts    3
  deliver_after   ""
  metadata        {}
```

---

## Processor → API: Status Stream

Published by the Notification Processor after each delivery attempt.
Consumed by the API service's status event consumer via consumer group `notify:cg:api`.

**Stream:** `notify:stream:status`

**Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `notification_id` | UUID string | yes | Matches the notification row in PostgreSQL |
| `status` | string | yes | `processing` \| `delivered` \| `failed` |
| `attempt_number` | integer string | yes | 1-indexed attempt counter |
| `http_status_code` | integer string | no | Provider HTTP response code; empty on network error |
| `error_message` | string | no | Human-readable failure reason; empty on success |
| `provider_message_id` | string | no | Provider-assigned message ID; populated on success |
| `latency_ms` | integer string | yes | Time from dispatch to provider response in milliseconds |
| `updated_at` | RFC3339 string | yes | Timestamp of the attempt |

**Example (successful delivery):**
```
XADD notify:stream:status *
  notification_id     550e8400-e29b-41d4-a716-446655440000
  status              delivered
  attempt_number      1
  http_status_code    202
  error_message       ""
  provider_message_id d4f8a3c1-9e2b-47f0-b3d2-8a1e6c5f0917
  latency_ms          143
  updated_at          2024-06-01T09:00:01Z
```

**Example (failed attempt, will retry):**
```
XADD notify:stream:status *
  notification_id     550e8400-e29b-41d4-a716-446655440000
  status              processing
  attempt_number      2
  http_status_code    503
  error_message       "provider returned 503 Service Unavailable"
  provider_message_id ""
  latency_ms          2014
  updated_at          2024-06-01T09:01:05Z
```

**Example (terminal failure — exhausted retries or non-retryable):**
```
XADD notify:stream:status *
  notification_id     550e8400-e29b-41d4-a716-446655440000
  status              failed
  attempt_number      4
  http_status_code    500
  error_message       "max attempts exhausted"
  provider_message_id ""
  latency_ms          1873
  updated_at          2024-06-01T09:08:30Z
```

---

## API Status Consumer Behaviour

On each `notify:stream:status` message, the API service consumer:

1. Parses all fields from the stream entry
2. Inserts a row into `delivery_attempts` (idempotent via `ON CONFLICT DO NOTHING` on `notification_id + attempt_number`)
3. Updates `notifications.status` and `notifications.updated_at`
   - `status=processing` → sets `notifications.status = 'processing'`
   - `status=delivered` → sets `notifications.status = 'delivered'`
   - `status=failed` → sets `notifications.status = 'failed'`
4. ACKs the message: `XACK notify:stream:status notify:cg:api {msg_id}`

Step 2 uses `ON CONFLICT DO NOTHING` so re-processing a duplicate status event is safe.
