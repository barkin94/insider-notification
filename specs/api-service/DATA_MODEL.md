# DATA MODEL — Notification System (PostgreSQL)

## Driver & Libraries
- Driver: `github.com/jackc/pgx/v5` + `pgxpool` for connection pooling
- Struct scanning: `github.com/georgysavva/scany/v2/pgxscan`
- All IDs use `UUID` (`gen_random_uuid()` on insert)
- All timestamps stored as `TIMESTAMPTZ` (UTC)
- Migrations managed by `golang-migrate`; files live in `internal/db/migrations/`

---

## Table: notifications

```sql
CREATE TABLE notifications (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id            UUID,
    recipient           VARCHAR(255) NOT NULL,
    channel             VARCHAR(20)  NOT NULL CHECK (channel IN ('sms', 'email', 'push')),
    content             TEXT         NOT NULL,
    priority            VARCHAR(20)  NOT NULL DEFAULT 'normal'
                            CHECK (priority IN ('high', 'normal', 'low')),
    status              VARCHAR(20)  NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'processing', 'delivered', 'failed', 'cancelled')),
    idempotency_key     VARCHAR(255) UNIQUE,
    deliver_after       TIMESTAMPTZ,
    provider_message_id VARCHAR(255),
    attempts            INT          NOT NULL DEFAULT 0,
    max_attempts        INT          NOT NULL DEFAULT 4,
    metadata            JSONB,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
```

**Go struct:**
```go
type Notification struct {
    ID                 uuid.UUID          `db:"id"`
    BatchID            *uuid.UUID         `db:"batch_id"`
    Recipient          string             `db:"recipient"`
    Channel            string             `db:"channel"`
    Content            string             `db:"content"`
    Priority           string             `db:"priority"`
    Status             string             `db:"status"`
    IdempotencyKey     *string            `db:"idempotency_key"`
    DeliverAfter       *time.Time         `db:"deliver_after"`
    ProviderMessageID  *string            `db:"provider_message_id"`
    Attempts           int                `db:"attempts"`
    MaxAttempts        int                `db:"max_attempts"`
    Metadata           []byte             `db:"metadata"` // raw JSONB bytes
    CreatedAt          time.Time          `db:"created_at"`
    UpdatedAt          time.Time          `db:"updated_at"`
}
```

**Status values:** `pending` | `processing` | `delivered` | `failed` | `cancelled`

**Field constraints:**
- `recipient`: max 255 chars, required
- `content`: max 1600 chars for SMS, 100,000 for Email, 4096 for Push (enforced at API layer)
- `priority`: defaults to `normal`
- `max_attempts`: always 4

**Indexes:**
```sql
CREATE INDEX idx_notifications_batch_id    ON notifications(batch_id) WHERE batch_id IS NOT NULL;
CREATE INDEX idx_notifications_status      ON notifications(status);
CREATE INDEX idx_notifications_channel     ON notifications(channel);
CREATE INDEX idx_notifications_created_at  ON notifications(created_at DESC);
CREATE INDEX idx_notifications_deliver_after_status ON notifications(deliver_after, status);
CREATE INDEX idx_notifications_status_updated_at    ON notifications(status, updated_at);
```

---

## Table: delivery_attempts

```sql
CREATE TABLE delivery_attempts (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id   UUID        NOT NULL REFERENCES notifications(id),
    attempt_number    INT         NOT NULL,
    status            VARCHAR(20) NOT NULL CHECK (status IN ('success', 'failed')),
    http_status_code  INT,
    provider_response JSONB,
    error_message     TEXT,
    latency_ms        INT,
    attempted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Go struct:**
```go
type DeliveryAttempt struct {
    ID               uuid.UUID  `db:"id"`
    NotificationID   uuid.UUID  `db:"notification_id"`
    AttemptNumber    int        `db:"attempt_number"`
    Status           string     `db:"status"`
    HTTPStatusCode   *int       `db:"http_status_code"`
    ProviderResponse []byte     `db:"provider_response"` // raw JSONB bytes
    ErrorMessage     *string    `db:"error_message"`
    LatencyMs        *int       `db:"latency_ms"`
    AttemptedAt      time.Time  `db:"attempted_at"`
}
```

**Indexes:**
```sql
CREATE INDEX idx_delivery_attempts_notification_id ON delivery_attempts(notification_id);
CREATE INDEX idx_delivery_attempts_attempted_at    ON delivery_attempts(attempted_at DESC);
```

---

## Table: idempotency_keys

```sql
CREATE TABLE idempotency_keys (
    key             VARCHAR(255) PRIMARY KEY,
    notification_id UUID         NOT NULL REFERENCES notifications(id),
    key_type        VARCHAR(20)  NOT NULL CHECK (key_type IN ('client', 'content_hash')),
    expires_at      TIMESTAMPTZ  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
```

**Go struct:**
```go
type IdempotencyKey struct {
    Key            string    `db:"key"`
    NotificationID uuid.UUID `db:"notification_id"`
    KeyType        string    `db:"key_type"`
    ExpiresAt      time.Time `db:"expires_at"`
    CreatedAt      time.Time `db:"created_at"`
}
```

**TTL rules:**
- `client` key type: expires 24h after creation
- `content_hash` key type: expires 1h after creation
- A background goroutine in the API service runs `DELETE FROM idempotency_keys WHERE expires_at < NOW()` every hour

**Index:**
```sql
CREATE INDEX idx_idempotency_keys_expires_at ON idempotency_keys(expires_at);
```

---

## Redis Key Patterns

```
# Priority streams (Redis Streams)
notify:stream:high              → Stream; values are notification UUIDs + deliver_after
notify:stream:normal            → Stream; values are notification UUIDs + deliver_after
notify:stream:low               → Stream; values are notification UUIDs + deliver_after

# Status event stream (Processor → API)
notify:stream:status            → Stream; delivery outcome events from Processor

# Consumer groups
notify:cg:processor             → Consumer group on priority streams (Processor workers)
notify:cg:api                   → Consumer group on status stream (API status consumer)

# Processing lock (prevents double-processing within Processor)
notify:lock:{notification_id}   → "1", TTL: 60s

# Rate limiter (token bucket per channel)
ratelimit:sms                   → Hash { tokens, last_refill }
ratelimit:email                 → Hash { tokens, last_refill }
ratelimit:push                  → Hash { tokens, last_refill }

# Idempotency fast path (client-supplied key only)
idempotency:{key}               → notification_id UUID string, TTL: 24h

# Metrics counters
metrics:sent:{channel}          → integer
metrics:failed:{channel}        → integer
metrics:queue_depth:{priority}  → integer (reconciled against XLEN on startup)
```

---

## Migration Files

Managed by `golang-migrate`. Files in `internal/db/migrations/`:

```
000001_create_notifications.up.sql
000001_create_notifications.down.sql
000002_create_delivery_attempts.up.sql
000002_create_delivery_attempts.down.sql
000003_create_idempotency_keys.up.sql
000003_create_idempotency_keys.down.sql
```

Each `.down.sql` file drops the objects created by its corresponding `.up.sql`.

---

## Atomic Update Patterns

Use `UPDATE ... WHERE status = $expected RETURNING *` for safe concurrent status transitions:

```go
row := pool.QueryRow(ctx, `
    UPDATE notifications
    SET    status = $1, updated_at = NOW()
    WHERE  id = $2 AND status = $3
    RETURNING id, status, updated_at
`, "processing", id, "pending")
// pgx.ErrNoRows → another worker already transitioned this row — skip
```

Use a **PostgreSQL transaction** for batch creation where `batch_id` assignment and multiple inserts must be atomic.

---

## Status Transition Map

```
    [pending] ──── XADD to stream ────► [processing]
                                          /         \
                                provider 202        provider error
                                    │                     │
                                    ▼                     ▼
                              [delivered]         attempts < max?
                                                   /           \
                                                 yes            no
                                                  │             │
                                           re-enqueue       [failed]
                                           (backoff delay)

    [pending] ──── cancel API ────► [cancelled]
```

---

## Content Validation Rules

| Channel | Max Length | Required Fields |
|---------|-----------|-----------------|
| sms     | 1600 chars | recipient (E.164 format), content |
| email   | 100,000 chars | recipient (valid email), content |
| push    | 4096 chars | recipient (device token), content |
