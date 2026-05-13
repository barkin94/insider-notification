# TASKS INDEX

Date: 2026-05-12
Source: specs/system/TODOS.md §§ Processor Database, Scheduled Notifications

## Sequence

```text
remove-processing-status ──────────────────┐
                                            │
processor-postgres                          │
  └── processor-db-package ────────────────►├──► processor-worker-attempts
                                            │          └── api-remove-attempts
scheduled-api
  └── scheduled-worker
```

`processor-worker-attempts` depends on both `processor-db-package` and
`remove-processing-status` (both touch `worker.go`).
The two chains (processor-db and scheduled) are otherwise independent.

## Status

- [x] remove-processing-status
- [x] processor-postgres
- [x] processor-db-package
- [x] processor-worker-attempts
- [x] api-remove-attempts
- [x] scheduled-api
- [x] scheduled-worker
