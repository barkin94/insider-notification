# scaffold

**Specs:** `system/ARCHITECTURE.md`
**Verification:** `system/VERIFICATION.md` § Scaffold
**Status:** complete

## What to build

| File | Notes |
|------|-------|
| `go.mod` | module `github.com/barkin/insider-notification`, go 1.23 |
| `api/main.go` | stub `main()` — log "starting api", exit 0 |
| `processor/main.go` | stub `main()` — log "starting processor", exit 0 |
| `Dockerfile.api` | multi-stage: build → distroless; exposes 8080 |
| `Dockerfile.processor` | multi-stage: build → distroless |
| `docker-compose.yml` | services: postgres, redis, api, processor, otel-collector, prometheus, grafana, jaeger |
| `Makefile` | targets: build, test, lint, migrate-up, migrate-down, docker-up, swag |
| `.env.example` | all env vars with descriptions and defaults |

## docker-compose services

| Service | Image | Ports |
|---------|-------|-------|
| postgres | postgres:16 | 5432 |
| redis | redis:7 | 6379 |
| api | ./Dockerfile.api | 8080 |
| processor | ./Dockerfile.processor | 8081 (metrics only) |
| otel-collector | otel/opentelemetry-collector-contrib | 4317 (OTLP gRPC) |
| prometheus | prom/prometheus | 9090 |
| grafana | grafana/grafana | 3000 |
| jaeger | jaegertracing/all-in-one | 16686, 14268 |

## .env.example vars

```
DATABASE_URL=postgres://postgres:postgres@localhost:5432/notifications?sslmode=disable
REDIS_ADDR=localhost:6379
PORT=8080
PROCESSOR_METRICS_PORT=8081
WORKER_CONCURRENCY=10
LOG_LEVEL=info
OTEL_ENDPOINT=localhost:4317
WEBHOOK_URL=https://webhook.site/<your-uuid>
```

## Tests

None — infrastructure only. Verified by `go build ./...` and `docker-compose config`.
