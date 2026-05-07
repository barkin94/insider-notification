# observability

**Specs:** `system/OBSERVABILITY.md`
**Verification:** `system/VERIFICATION.md` § Observability
**Status:** pending

## What to build

| File | Purpose |
|------|---------|
| `otel-collector-config.yaml` | OTLP gRPC receiver → Jaeger exporter + Prometheus exporter |
| `prometheus.yml` | Scrape api:8080/metrics and processor:8081/metrics every 15s |
| `grafana/provisioning/datasources/prometheus.yaml` | Auto-provision Prometheus datasource |
| `grafana/provisioning/dashboards/notification.json` | Dashboard: queue depth, delivery rate, latency histogram, failed counter |

## OTel instrumentation (added to api-main and processor-main)

Both services initialise the OTel SDK with:
- `go.opentelemetry.io/otel/exporters/prometheus` → metrics on `/metrics`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` → traces to OTel Collector

Metrics registered per `OBSERVABILITY.md`:

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
- Jaeger shows spans for HTTP request → DB query → stream publish
- Grafana dashboard loads with data
