# shared-config

**Specs:** `system/ARCHITECTURE.md`
**Verification:** `system/VERIFICATION.md` § Scaffold (`go build ./...` passes)
**Status:** pending

## What to build

### `internal/shared/config/config.go`

```
Config struct:
  DatabaseURL          string  (env: DATABASE_URL)
  RedisAddr            string  (env: REDIS_ADDR)
  Port                 int     (env: PORT, default: 8080)
  ProcessorMetricsPort int     (env: PROCESSOR_METRICS_PORT, default: 8081)
  WorkerConcurrency    int     (env: WORKER_CONCURRENCY, default: 10)
  LogLevel             string  (env: LOG_LEVEL, default: "info")
  OTelEndpoint         string  (env: OTEL_ENDPOINT)
  WebhookURL           string  (env: WEBHOOK_URL)

Load() (*Config, error)
  — reads from environment via viper
  — returns error if DatabaseURL or RedisAddr is empty
```

## Tests

`internal/shared/config/config_test.go`

- `TestLoad_defaults` — unset optional vars → defaults applied
- `TestLoad_fromEnv` — all vars set via t.Setenv → struct populated correctly
- `TestLoad_missingRequired` — missing DATABASE_URL → error returned
