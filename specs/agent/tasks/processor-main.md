# processor-main

**Specs:** `system/ARCHITECTURE.md`, `system/OBSERVABILITY.md`
**Verification:** `system/VERIFICATION.md` § Processor Main
**Status:** pending

## What to build

### `processor/main.go`

Startup sequence:
1. `config.Load()` → Config
2. Init slog logger
3. Init OTel SDK: Prometheus metrics exporter + OTLP trace exporter → OTel Collector
4. `redis.NewClient(cfg.RedisAddr)` → redis client
5. `db.NewPool(ctx, cfg.DatabaseURL)` → pgxpool
6. Construct repositories (notification, delivery attempt)
7. Construct stream consumer (`notify:cg:processor`) + producer
8. Run PEL reclaim: `consumer.ReclaimStale(ctx, each priority stream, 2*time.Minute)`
9. Construct rate limiter, delivery client, retry backoff
10. Start `WORKER_CONCURRENCY` worker goroutines, each calling `worker.Run(ctx)`
11. Expose Prometheus metrics on `cfg.ProcessorMetricsPort`
12. On SIGINT/SIGTERM: cancel ctx, wait for all workers to finish

## Tests

- `go build ./processor` passes (build-time verification)
