# refactor

**Status:** pending

## Context

Scope: **API service only** (`api/internal/handler/`, `api/internal/service/`, `api/internal/db/`).
After the API is fully built and all tests pass, do a structured review pass to find and fix
areas worth cleaning up. This is not a "change everything" pass — only act on clear improvements.

## Areas to inspect

### Duplication
- `service.Create` and `service.createWithBatchID` share most logic — consolidate into one private method.
- Any repeated error-mapping patterns in handlers (consider a shared helper).

### Interface boundaries
- Review which interfaces are too wide (expose methods the consumer never calls).
- Review which structs leak implementation details across package boundaries.

### Error handling
- Confirm every error returned to the handler is either a `ValidationError`, a sentinel
  (`db.ErrNotFound`, `db.ErrTransitionFailed`), or wrapped with `fmt.Errorf`. No raw errors.
- Confirm no error is silently swallowed (search for `_ =` on error returns).

### Test quality
- Check for tests that assert implementation details rather than behaviour.
- Confirm mock function fields that are never called in a given test don't panic on nil.

### Exported symbols
- Check for any unnecessarily exported types or functions used only within `api/internal/`.

## Deliverable

A commit (or small series of commits) with targeted cleanups. Each change must have a test that
passes before and after. No behaviour changes — refactor only.
