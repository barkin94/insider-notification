# observability

**Specs:** `system/OBSERVABILITY.md`
**Verification:** `system/VERIFICATION.md` § Observability
**Status:** pending

## What to build

### Infrastructure config files

| File | Purpose |
|------|---------|
| `otel-collector-config.yaml` | OTLP gRPC receiver → Jaeger exporter + Prometheus exporter |
| `prometheus.yml` | Scrape api:8080/metrics and processor:8081/metrics every 15s |
| `grafana/provisioning/datasources/prometheus.yaml` | Auto-provision Prometheus datasource |
| `grafana/provisioning/dashboards/notification.json` | Dashboard: queue depth, delivery rate, latency histogram, failed counter |

### OTel SDK initialisation (added to api-main and processor-main)

Both services initialise the OTel SDK with:
- `go.opentelemetry.io/otel/exporters/prometheus` → metrics on `/metrics`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` → traces to OTel Collector

### slog integration (both services)

All log calls in the codebase use package-level `slog.InfoContext(ctx, ...)` / `slog.ErrorContext`
etc. — no logger fields in structs, no logger injection. `slog.SetDefault` in each `main()` is
the single wiring point:

```go
bridge := otelslog.NewHandler("api") // go.opentelemetry.io/contrib/bridges/otelslog
logger := slog.New(slog.NewMultiHandler(
    slog.NewJSONHandler(os.Stdout, nil),
    bridge,
))
slog.SetDefault(logger)
```

After this, every `slog.*Context(ctx, ...)` call anywhere in the codebase automatically:
- Writes structured JSON to stdout
- Ships the record to the OTel Collector with `trace_id`/`span_id` injected from the active span in ctx

No other code changes needed — all log calls already pass ctx.

### OTel HTTP middleware (API service)

Wrap the router with `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` so every
incoming request creates a span automatically. The active span is placed on `r.Context()`,
which flows through to all `slog.*Context` calls automatically.

### Trace context propagation through Redis Streams

`internal/shared/stream/carrier.go`:
```
StreamCarrier struct{ values map[string]any }  ← implements propagation.TextMapCarrier
  Get(key string) string
  Set(key string, value string)
  Keys() []string
```

**API side** — before publishing a priority message, inject current span context:
```go
otel.GetTextMapPropagator().Inject(ctx, &StreamCarrier{values: msgValues})
```
This populates `traceparent` and `tracestate` fields in the stream message.

**Processor side** — after reading a priority message, extract span context and start child span:
```go
ctx = otel.GetTextMapPropagator().Extract(ctx, &StreamCarrier{values: msg.Values})
ctx, span := tracer.Start(ctx, "processor.deliver")
defer span.End()
```

### Metrics registered

| Metric | Service |
|--------|---------|
| `notification.sent` Counter | Processor |
| `notification.failed` Counter | Processor |
| `notification.attempts` Counter | Processor |
| `notification.delivery.latency_ms` Histogram | Processor |
| `notification.queue.depth` Gauge | API (XLEN on each priority stream) |
| `ratelimiter.tokens` Gauge | Processor |

## Tests

Verified manually after `docker-compose up`:
- Prometheus `/graph` shows all 6 metrics
- Jaeger shows a unified trace: `POST /notifications` → `processor.deliver`
- Grafana dashboard loads with data
