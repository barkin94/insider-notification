# api-main

**Specs:** `system/ARCHITECTURE.md`, `system/OBSERVABILITY.md`
**Verification:** `system/VERIFICATION.md` § API Main
**Status:** pending

## What to build

### `api/main.go`

Startup sequence:
1. `config.Load()` → Config
2. Init slog logger (level from config)
3. Init OTel SDK: Prometheus metrics exporter + OTLP trace exporter → OTel Collector
4. `db.NewPool(ctx, cfg.DatabaseURL)` → pgxpool
5. Run migrations via golang-migrate
6. `redis.NewClient(cfg.RedisAddr)` → redis client
7. Construct repositories (notification, delivery attempt, idempotency)
8. Construct stream producer + consumer
9. Construct idempotency checker; start cleanup goroutine
10. Construct status consumer; start `Run(ctx)` goroutine
11. Build chi router with all handlers
12. `http.ListenAndServe(cfg.Port, router)`
13. On SIGINT/SIGTERM: cancel ctx, drain in-flight requests (5s timeout), close pool

## Tests

- `go build ./api` passes (build-time verification)
- Integration: start API against testcontainers postgres + redis; `GET /health` returns 200
