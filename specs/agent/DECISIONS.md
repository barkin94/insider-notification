# DECISIONS

Patterns and decisions that emerged during implementation. Updated after each phase.
Empty until implementation begins.

---

## Established Patterns

- **UUID v7 for all IDs** — app-generated via `uuid.NewV7()` before every INSERT. No `gen_random_uuid()` default in migrations. Rationale: time-ordered UUIDs give better B-tree index locality on insert-heavy tables; v7 supported by `github.com/google/uuid` v1.6.0 without any PostgreSQL extension.

- **Processor is PostgreSQL-free** — full notification payload (channel, recipient, content, priority, attempt_number, max_attempts, deliver_after, metadata) is embedded in each priority stream message. The Processor never queries PostgreSQL. Repositories live in `api/internal/db/`, not `internal/shared/db/`. Cancellation is signalled via a Redis key (`cancelled:{id}`, TTL 24h) set by the API.

## Open Spec Decisions

_None yet._

## Explicitly Not Done

_None yet._
