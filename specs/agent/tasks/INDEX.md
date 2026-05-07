# TASKS INDEX
Date: 2026-05-07
Session: first build — full spec suite

## Sequence

```
scaffold
  └── shared-models
        └── shared-config
              └── sql-migrations
                    └── postgres-repos
                          └── stream-adapter
                                ├── idempotency ──────────────────┐
                                ├── api-middleware                 │
                                ├── rate-limiter                   │
                                ├── retry                          │
                                └── delivery-client                │
                                      ├── api-handlers ◄───────────┤
                                      ├── processor-worker          │
                                      └── status-consumer ◄────────┘
                                            ├── api-main
                                            └── processor-main
                                                  └── observability
                                                        └── docs-readme
```

## Status

- [x] scaffold
- [x] shared-models
- [x] shared-config
- [x] sql-migrations
- [x] postgres-repos
- [ ] stream-adapter
- [ ] idempotency
- [ ] api-middleware
- [ ] rate-limiter
- [ ] retry
- [ ] delivery-client
- [ ] api-handlers
- [ ] processor-worker
- [ ] status-consumer
- [ ] api-main
- [ ] processor-main
- [ ] observability
- [ ] docs-readme
