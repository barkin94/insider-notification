# api-middleware

**Specs:** `system/OBSERVABILITY.md`
**Verification:** `api-service/VERIFICATION.md` § Middleware
**Status:** complete

## What to build

### `api/internal/middleware/logger.go`
```
Logger(logger *slog.Logger) func(http.Handler) http.Handler
  — logs each request: method, path, status, latency_ms
  — trace_id field added when OTel HTTP middleware is wired (observability task)
```

Note: X-Correlation-ID middleware was considered and removed. OTel trace context
(traceparent header + trace propagation through stream messages) serves this
purpose. See observability task for OTel HTTP middleware setup.

## Tests

`api/internal/middleware/logger_test.go`:
- `TestLogger_fields` — log output contains required fields (time, level, msg, method, path, status, latency_ms)
