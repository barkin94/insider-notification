# shared-config

**Specs:** `system/ARCHITECTURE.md`
**Verification:** `system/VERIFICATION.md` § Scaffold (`go build ./...` passes)
**Status:** complete

## What to build

### `internal/shared/config/config.go`
Exports `Base` struct (DatabaseURL, RedisAddr, LogLevel, OTelEndpoint), `NewViper()`, and `LoadBase()`.

### `api/internal/config/config.go`
`Config` embeds `shared.Base` + `Port` (default 8080). `Load()` calls `shared.LoadBase`.

### `processor/internal/config/config.go`
`Config` embeds `shared.Base` + `MetricsPort` (default 8081), `WorkerConcurrency` (default 10), `WebhookURL` (required). `Load()` calls `shared.LoadBase`.

## Tests

None — config loading is infrastructure wiring, verified by running the stack.
