# VERIFICATION — Notification System

Per-component checklists:
- [API Service](../api-service/VERIFICATION.md)
- [Processor Service](../processor-service/VERIFICATION.md)

---

## Scaffold

- [ ] `docker-compose config` passes with no errors
- [ ] `go build ./...` passes for both `api/` and `processor/` entrypoints
- [ ] All directories from `ARCHITECTURE.md` project layout exist
- [ ] `.env.example` present with all required env vars documented
- [ ] `golang-migrate` runs cleanly; all migration files apply and reverse without error

---

## API Main

- [ ] OTel SDK initialised; traces appear in Jaeger after `docker-compose up`
- [ ] Router mounts all handlers and middleware
- [ ] Status event consumer goroutine starts on boot
- [ ] Graceful shutdown: in-flight requests drain before exit
- [ ] `go build ./api` passes

---

## Processor Main

- [ ] OTel SDK initialised; traces appear in Jaeger after `docker-compose up`
- [ ] 10 worker goroutines start; startup log confirms worker count
- [ ] PEL reclaim runs on boot before workers begin polling
- [ ] Graceful shutdown: active workers finish current message before exit
- [ ] `go build ./processor` passes

---

## Observability

- [ ] Prometheus scrapes `/metrics` on both services; all metrics from `OBSERVABILITY.md` visible
- [ ] Grafana dashboard loads at `http://localhost:3000`
- [ ] Jaeger UI shows end-to-end trace for a single notification (`http://localhost:16686`)
- [ ] `notification.queue.depth` gauge updates when messages are enqueued

---

## Tests

- [ ] `go test ./...` passes with zero failures
- [ ] `go test -race ./...` clean (no data races)
- [ ] Retry exhaustion: 4 failed attempts → `notifications.status = failed`
- [ ] Non-retryable: provider 400 → `failed` immediately (no retry)
- [ ] End-to-end: `POST /notifications` → stream consumed → provider 202 → `GET /notifications/:id` returns `status: delivered`

---

## Docs

- [ ] `swag init -dir api` generates `docs/` without error
- [ ] Swagger UI accessible at `http://localhost:8080/swagger` after `docker-compose up`
- [ ] README.md contains: architecture diagram, quick-start commands, API examples, key design decisions

---

## Done Criteria

- [ ] `docker-compose up` starts all services with no errors
- [ ] `make test` passes all tests
- [ ] `GET /health` returns `{"status": "ok"}`
- [ ] `POST /notifications` → PostgreSQL record `pending` → Processor delivers → `status: delivered`
- [ ] Grafana shows `notification.sent` counter increment; Jaeger shows end-to-end trace
- [ ] Duplicate `Idempotency-Key` header → 409
- [ ] Swagger UI accessible at `http://localhost:8080/swagger`
- [ ] Commit history is clean and atomic
