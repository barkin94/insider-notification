# sql-migrations

**Specs:** `api-service/DATA_MODEL.md`
**Verification:** `api-service/VERIFICATION.md` § Data Layer
**Status:** pending

## What to build

Three migration pairs under `api/migrations/`:

| File | Creates |
|------|---------|
| `000001_create_notifications.up.sql` | `notifications` table + all indexes |
| `000001_create_notifications.down.sql` | `DROP TABLE notifications CASCADE` |
| `000002_create_delivery_attempts.up.sql` | `delivery_attempts` table + all indexes |
| `000002_create_delivery_attempts.down.sql` | `DROP TABLE delivery_attempts CASCADE` |
| `000003_create_idempotency_keys.up.sql` | `idempotency_keys` table + index |
| `000003_create_idempotency_keys.down.sql` | `DROP TABLE idempotency_keys CASCADE` |

Each up migration must include every column, constraint, and index from `DATA_MODEL.md` exactly — including the `UNIQUE INDEX` on `delivery_attempts(notification_id, attempt_number)`.

## Tests

None — verified by running `golang-migrate` in the scaffold verification step and in `postgres-repos` testcontainers tests.
