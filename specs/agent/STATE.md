---
status: in_progress
last_completed: Phase 0 (d52541d)
current_phase: Phase 1
branch: spec-restructure
---

# STATE

## current session

Migrating spec structure from topic-based to component-based (one file per code unit). Phase 0 complete: INSTRUCTIONS.md, STANDARDS.md, STATE.md rewritten. Starting Phase 1: create shared specs (shared/model.md, shared/stream.md, shared/observability.md).

## divergences (spec vs code)

Known stale specs to be fixed during migration:

- `specs/system/OBSERVABILITY.md` and `specs/processor-service/OBSERVABILITY.md` are redundant; will consolidate into `specs/shared/observability.md`
- `specs/api-service/DATA_MODEL.md` lists `delivery_attempts` table, but processor now owns it (will split into separate repo specs)
- Package paths in old CODEINDEX.md use `internal/shared/` but actual paths are `shared/` (will fix during migration)

## TODO (in order)

- [ ] Phase 1: Create shared specs (shared/model.md, shared/stream.md, shared/observability.md)
- [ ] Phase 2: Create API specs (api/handler.notifications.md, api/service.notification.md, api/repo.notification.md, api/repo.idempotency.md)
- [ ] Phase 3: Create Processor specs (processor/worker.md, processor/scheduler.md, processor/priorityrouter.md, processor/repo.delivery_attempt.md)
- [ ] Phase 4: Create infra specs (infra/compose.md, infra/config.md)
- [ ] Phase 5: Create deferred specs (deferred/websocket.md, deferred/templates.md, deferred/ci.md)
- [ ] Phase 6: Delete old specs (system/, api-service/, processor-service/ directories and agent/tasks/, agent/CODEINDEX.md, agent/DECISIONS.md)
- [ ] fix-processor-db-test (related: processor/repo.delivery_attempt.md, will reference the new spec)
