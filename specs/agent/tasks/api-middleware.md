# api-middleware

**Specs:** `system/OBSERVABILITY.md`
**Verification:** `api-service/VERIFICATION.md` § Middleware
**Status:** pending

## What to build

### `api/internal/middleware/correlation.go`
```
CorrelationID(next http.Handler) http.Handler
  — reads X-Correlation-ID header; generates UUID v4 if absent
  — stores in request context
  — writes to response header

FromContext(ctx) string  ← retrieves correlation ID from context
```

### `api/internal/middleware/logger.go`
```
Logger(logger *slog.Logger) func(http.Handler) http.Handler
  — logs each request: method, path, status, latency, correlation_id
  — required log fields: ts, level, msg, service, version (from OBSERVABILITY.md)
```

## Tests

`api/internal/middleware/correlation_test.go`:
- `TestCorrelationID_generated` — absent header → UUID generated and set in response
- `TestCorrelationID_propagated` — present header → same value echoed in response
- `TestCorrelationID_inContext` — handler can retrieve ID via FromContext

`api/internal/middleware/logger_test.go`:
- `TestLogger_fields` — log output contains required fields (ts, level, msg, service, version)
