# observability

**Specs:** `system/OBSERVABILITY.md`
**Verification:** `system/VERIFICATION.md` § Observability
**Status:** done

## What was built

### Infrastructure config files

| File | Purpose |
|------|---------|
| `otel-collector-config.yaml` | OTLP gRPC receiver → Tempo exporter + Prometheus exporter |
| `tempo.yaml` | Grafana Tempo config: OTLP gRPC receiver, local trace storage |
| `prometheus.yml` | Scrapes api:8080/metrics and processor:8081/metrics every 15s |
| `grafana/provisioning/datasources/prometheus.yaml` | Auto-provision Prometheus datasource |
| `grafana/provisioning/datasources/tempo.yaml` | Auto-provision Tempo datasource |
| `grafana/provisioning/dashboards/notification.json` | Dashboard: queue depth, delivery rate, latency histogram, failed counter |

### OTel SDK initialisation (`shared/otel/setup.go`)

`Init(ctx, serviceName, collectorEndpoint string) (Shutdown, error)` sets up:
- `TracerProvider` — OTLP gRPC exporter → OTel Collector → Tempo
- `MeterProvider` — Prometheus exporter → default prometheus registry → `/metrics`
- `propagation.TraceContext{}` as the global text map propagator (W3C Trace Context only; Baggage not used)

Both global providers are set via `otel.SetTracerProvider` / `otel.SetMeterProvider`.
Returns a `Shutdown func(context.Context) error` to flush and stop both providers on exit.

### slog multi-handler (`shared/otel/multihandler.go`)

`NewMultiHandler(handlers ...slog.Handler) slog.Handler` fans out every log record to all
handlers. Used in `initLogger` in both mains to write to two destinations simultaneously:

```go
slog.SetDefault(slog.New(sharedotel.NewMultiHandler(
    slog.NewJSONHandler(os.Stdout, opts),  // structured JSON to stdout
    otelslog.NewHandler(serviceName),      // ships record to OTel Collector with trace_id/span_id
)))
```

Note: `slog.NewMultiHandler` does not exist in the Go stdlib — `sharedotel.NewMultiHandler`
is our own implementation.

Loki is required to collect stdout logs and make log-trace correlation work in Grafana.
Adding Loki + Promtail to docker-compose is a pending infrastructure task.

### Trace context propagation (`shared/stream/carrier.go`)

`StreamCarrier` implements `propagation.TextMapCarrier` over Watermill `message.Metadata`
so W3C `traceparent` is stamped into Redis Stream messages on publish and extracted on consume.

**API side** (`shared/stream/publisher.go`) — before every publish:
```go
otel.GetTextMapPropagator().Inject(ctx, &StreamCarrier{metadata: msg.Metadata})
```

**Processor side** — after reading a message, extract and start a child span (implemented in the notification-processing task).

### OTel HTTP middleware (API service)

`api/internal/handler/router.go` wraps the chi router with `otelhttp.NewHandler` so every
incoming HTTP request creates a root span automatically. The span is placed on `r.Context()`,
flowing through to all downstream `slog.*Context` calls.

### Metrics endpoint

- **API** (`api/cmd/main.go`): `/metrics` route added to the chi router via `promhttp.Handler()`
- **Processor** (`processor/cmd/main.go`): dedicated HTTP server on `cfg.MetricsPort` (default 8081) serving `/metrics`

### Metrics registered

Metric instruments are registered in the notification-processing task (when `processOne` is implemented):

| Metric | Type | Service |
|--------|------|---------|
| `notification.sent` | Counter | Processor |
| `notification.failed` | Counter | Processor |
| `notification.attempts` | Counter | Processor |
| `notification.delivery.latency_ms` | Histogram | Processor |
| `notification.queue.depth` | Gauge | API |
| `ratelimiter.tokens` | Gauge | Processor |

## Verification

After `docker-compose up`:
- Prometheus `http://localhost:9090/graph` shows all 6 metrics
- Grafana `http://localhost:3000` — Tempo datasource shows traces; dashboard loads with data
- Jaeger is not used; traces visible in Grafana via Tempo
