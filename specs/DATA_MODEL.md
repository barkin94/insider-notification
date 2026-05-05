# DATA MODEL — Notification System (MongoDB)

## Driver & Library
- Driver: `go.mongodb.org/mongo-driver/mongo`
- All IDs use MongoDB's native `primitive.ObjectID` (stored as `_id`)
- External-facing IDs are serialized as hex strings
- All timestamps stored as `time.Time` (UTC)

---

## Collection: notifications

```go
type Notification struct {
  ID                primitive.ObjectID  `bson:"_id,omitempty"`
  BatchID           *primitive.ObjectID `bson:"batch_id,omitempty"`
  Recipient         string              `bson:"recipient"`
  Channel           string              `bson:"channel"`             // sms | email | push
  Content           string              `bson:"content"`
  Priority          string              `bson:"priority"`            // high | normal | low
  Status            string              `bson:"status"`              // see status values below
  IdempotencyKey    *string             `bson:"idempotency_key,omitempty"`
  DeliverAfter      *time.Time          `bson:"deliver_after,omitempty"`
  ProviderMessageID *string             `bson:"provider_message_id,omitempty"`
  Attempts          int                 `bson:"attempts"`
  MaxAttempts       int                 `bson:"max_attempts"`        // always 4
  Metadata          bson.M              `bson:"metadata,omitempty"`
  CreatedAt         time.Time           `bson:"created_at"`
  UpdatedAt         time.Time           `bson:"updated_at"`
}
```

**Status values:** `pending` | `processing` | `delivered` | `failed` | `cancelled`

**Field constraints:**
- `recipient`: max 255 chars, required
- `content`: max 1600 chars for SMS, 100,000 for Email, 4096 for Push
- `priority`: defaults to `normal`
- `max_attempts`: always 4

**Indexes:**
```js
db.notifications.createIndex({ batch_id: 1 }, { sparse: true })
db.notifications.createIndex({ status: 1 })
db.notifications.createIndex({ channel: 1 })
db.notifications.createIndex({ created_at: -1 })
db.notifications.createIndex({ deliver_after: 1, status: 1 })
db.notifications.createIndex({ idempotency_key: 1 }, { unique: true, sparse: true })
db.notifications.createIndex({ status: 1, updated_at: 1 })
```

---

## Collection: delivery_attempts

```go
type DeliveryAttempt struct {
  ID               primitive.ObjectID `bson:"_id,omitempty"`
  NotificationID   primitive.ObjectID `bson:"notification_id"`
  AttemptNumber    int                `bson:"attempt_number"`      // 1-indexed
  Status           string             `bson:"status"`              // success | failed
  HTTPStatusCode   *int               `bson:"http_status_code,omitempty"`
  ProviderResponse bson.M             `bson:"provider_response,omitempty"`
  ErrorMessage     *string            `bson:"error_message,omitempty"`
  LatencyMs        *int               `bson:"latency_ms,omitempty"`
  AttemptedAt      time.Time          `bson:"attempted_at"`
}
```

**Indexes:**
```js
db.delivery_attempts.createIndex({ notification_id: 1 })
db.delivery_attempts.createIndex({ attempted_at: -1 })
```

---

## Collection: idempotency_keys

```go
type IdempotencyKey struct {
  ID             primitive.ObjectID `bson:"_id,omitempty"`
  Key            string             `bson:"key"`
  NotificationID primitive.ObjectID `bson:"notification_id"`
  KeyType        string             `bson:"key_type"`    // client | content_hash
  ExpiresAt      time.Time          `bson:"expires_at"`
  CreatedAt      time.Time          `bson:"created_at"`
}
```

**TTL rules:**
- `client` key type: expires 24h after creation
- `content_hash` key type: expires 1h after creation
- MongoDB TTL index auto-deletes expired documents — no cleanup goroutine needed

**Indexes:**
```js
db.idempotency_keys.createIndex({ key: 1 }, { unique: true })
db.idempotency_keys.createIndex({ expires_at: 1 }, { expireAfterSeconds: 0 })
```

---

## Redis Key Patterns

```
# Priority queues (Redis Lists)
notify:queue:high             → List of notification ObjectID hex strings
notify:queue:normal           → List of notification ObjectID hex strings
notify:queue:low              → List of notification ObjectID hex strings

# Processing lock (prevents double-processing)
notify:lock:{notification_id} → "1", TTL: 60s

# Rate limiter (token bucket per channel)
ratelimit:sms                 → Hash { tokens, last_refill }
ratelimit:email               → Hash { tokens, last_refill }
ratelimit:push                → Hash { tokens, last_refill }

# Idempotency fast path (client key only)
idempotency:{key}             → notification_id hex string, TTL: 24h

# Metrics counters
metrics:sent:{channel}        → integer
metrics:failed:{channel}      → integer
metrics:queue_depth:{priority} → integer
```

---

## Migration Runner

MongoDB is schemaless so there are no SQL files. Use a lightweight Go-based migration runner:

```go
// internal/db/migrations/runner.go
type Migration struct {
  Version int
  Name    string
  Up      func(ctx context.Context, db *mongo.Database) error
}

// Tracks applied migrations in a `schema_migrations` collection:
type MigrationRecord struct {
  Version   int       `bson:"version"`
  Name      string    `bson:"name"`
  AppliedAt time.Time `bson:"applied_at"`
}
```

```go
// internal/db/migrations/migrations.go
var All = []Migration{
  {
    Version: 1,
    Name:    "create_notifications_indexes",
    Up: func(ctx context.Context, db *mongo.Database) error {
      // create all indexes for notifications collection
    },
  },
  {
    Version: 2,
    Name:    "create_delivery_attempts_indexes",
    Up: func(ctx context.Context, db *mongo.Database) error {
      // create indexes for delivery_attempts
    },
  },
  {
    Version: 3,
    Name:    "create_idempotency_keys_ttl_index",
    Up: func(ctx context.Context, db *mongo.Database) error {
      // create unique + TTL indexes for idempotency_keys
    },
  },
}
```

---

## Atomic Update Patterns

Use `FindOneAndUpdate` with a status guard for safe concurrent transitions:

```go
filter := bson.M{
  "_id":    id,
  "status": "pending",   // guard: only update if still in expected state
}
update := bson.M{
  "$set": bson.M{
    "status":     "processing",
    "updated_at": time.Now().UTC(),
  },
}
opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
result := col.FindOneAndUpdate(ctx, filter, update, opts)
// If result.Err() == mongo.ErrNoDocuments: another worker grabbed it — skip
```

Use **multi-document transactions** (requires replica set or mongos) for batch creation
where `batch_id` assignment and multiple inserts must be atomic.

---

## Status Transition Map

```
    [pending] ──── enqueued to Redis queue ────► [processing]
                                                  /         \
                                      provider 202          provider error
                                          │                      │
                                          ▼                      ▼
                                    [delivered]          attempts < max?
                                                          /           \
                                                        yes            no
                                                         │             │
                                                re-enqueue with    [failed]
                                                deliver_after
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
