# CHECKS

Read after completing each task.

After every task where a Go file changed: `go build ./...` and `go vet ./...` must pass.

| Task | Additional check |
|------|-----------------|
| 1.3 | `docker-compose config` |
| 1.5 | `go run ./cmd/migrate up` && `go run ./cmd/migrate status` |
| 1.6 | `go build ./internal/config/...` |
| 2.x | `go test ./internal/db/... -v -run TestUnit` |
| 3.1–3.2 | `go test ./internal/queue/... -v` |
| 3.3 | `go test ./internal/ratelimit/... -v` |
| 4.x | `go test ./internal/delivery/... ./internal/retry/... ./internal/service/... -v` |
| 5.x | `go test ./internal/api/... -v` |
| 6.x | `go test ./internal/api/ws/... -v` |
| 8.x | `go test ./... -v` |
| 9.1 | `swag init && ls docs/` |
| 9.3 | `cat .github/workflows/ci.yml` |

A task is complete only when every applicable check exits 0. If any check fails, set `BLOCKED_REASON` in `AGENT_STATE.md` and fix before marking complete.
