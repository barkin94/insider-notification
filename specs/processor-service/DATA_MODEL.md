# DATA MODEL — Processor Service (PostgreSQL)

The processor owns a dedicated `processor` schema inside the shared `notifications` PostgreSQL
instance. All IDs are UUIDs. All timestamps are stored in UTC with timezone.

Migrations are embedded SQL files (`*.up.sql` / `*.down.sql`) run programmatically at startup
via `bun/migrate`. The DSN sets `search_path=processor,public` so queries default to the
`processor` schema without prefix, while still being able to read `public.notifications`
(owned by the API service).

---

## Schema: processor

### Table: delivery_attempts

Durable audit log of every delivery attempt written by the worker after each webhook call.
Observability data (HTTP status code, latency) is emitted as OTel spans — see OBSERVABILITY.md.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | UUID | PRIMARY KEY |
| `notification_id` | UUID | NOT NULL |
| `attempt_number` | INT | NOT NULL, 1-indexed |
| `status` | VARCHAR(20) | NOT NULL, enum: `delivered \| failed` |
| `created_at` | TIMESTAMPTZ | NOT NULL, default now |
| `updated_at` | TIMESTAMPTZ | NOT NULL, default now |

**No FK to `public.notifications`** — cross-schema foreign keys would couple the two services
at the database level.

**Constraints:**
- `UNIQUE (notification_id, attempt_number)` — enables idempotent insert: if a worker crashes
  after writing but before acking, the re-delivered message hits `ON CONFLICT DO NOTHING`
  instead of creating a duplicate row.

**Indexes:**
```sql
CREATE INDEX idx_delivery_attempts_notification_id ON delivery_attempts(notification_id);
CREATE INDEX idx_delivery_attempts_created_at ON delivery_attempts(created_at DESC);
```

---

## External table access: public.notifications

The processor reads (but does not own) the `notifications` table from the `public` schema,
accessible via `search_path`.

| Operation | Query |
|-----------|-------|
| Scheduler poll | `SELECT … WHERE deliver_after IS NOT NULL AND deliver_after <= NOW() AND status = 'pending'` |

Write operations: none. Status updates flow back through the `notify:stream:status` Redis
stream, consumed by the API service's status consumer.
