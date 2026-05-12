# processor-postgres

**Specs:** `system/TODOS.md` § Processor Database
**Status:** complete

## Context

The Processor service has no database today. Rather than adding a new postgres container,
the Processor will own a dedicated `processor` schema inside the existing `notifications`
postgres instance. This gives clear logical separation with zero infra overhead; moving to
a separate postgres instance later is a DSN-only change.

## What to build

### `docker-compose.yml`
Add to the `processor` service environment:
```
DATABASE_URL: postgres://postgres:postgres@postgres:5432/notifications?sslmode=disable&search_path=processor,public
```
The `search_path=processor,public` ensures the processor's migrations and queries default
to the `processor` schema without needing to prefix every table name.

Add `postgres: condition: service_healthy` to the processor's `depends_on` (if not already present).

No new Docker services. No Go code changes in this task — the processor config already
inherits `DatabaseURL` from `shared.Base` and `LoadBase` already reads `DATABASE_URL`
from env.

## Verification

- `docker compose up --build processor` starts without error
- Processor container logs show no missing-env errors
