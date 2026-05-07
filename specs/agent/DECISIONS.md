# DECISIONS

Patterns and decisions that emerged during implementation. Updated after each phase.
Empty until implementation begins.

---

## Established Patterns

- **UUID v7 for all IDs** — app-generated via `uuid.NewV7()` before every INSERT. No `gen_random_uuid()` default in migrations. Rationale: time-ordered UUIDs give better B-tree index locality on insert-heavy tables; v7 supported by `github.com/google/uuid` v1.6.0 without any PostgreSQL extension.

## Open Spec Decisions

_None yet._

## Explicitly Not Done

_None yet._
