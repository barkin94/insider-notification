---
name: Code Index
description: Maps each spec to the packages that implement it. Read alongside STATE.md at session start to identify affected packages when a spec changes.
type: reference
---

# CODE INDEX

★ = cross-service: a change here affects both `api/` and `processor/`

| Spec | Packages |
|------|----------|
| `system/ARCHITECTURE.md` | `api/main.go`, `processor/main.go` |
| `system/MESSAGE_CONTRACT.md` | `internal/shared/stream/` ★ |
| `system/OBSERVABILITY.md` | `api/main.go`, `processor/main.go`, `internal/api/middleware/` ★ |
| `api-service/DATA_MODEL.md` | `internal/shared/model/`, `internal/shared/db/`, `api/migrations/` |
| `api-service/API_CONTRACT.md` | `internal/api/handler/`, `internal/api/middleware/` |
| `processor-service/QUEUE_DESIGN.md` | `internal/shared/stream/`, `internal/processor/worker/` ★ |
| `processor-service/RETRY_POLICY.md` | `internal/processor/worker/`, `internal/processor/retry/` |
