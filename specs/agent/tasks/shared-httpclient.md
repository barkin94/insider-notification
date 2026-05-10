# shared-httpclient

**Status:** complete

## Context

`processor/internal/worker/delivery/client.go` hardcodes `*http.Client` construction with a fixed
timeout and no transport configuration. The observability task will need to wrap the transport
with `otelhttp.NewTransport` for tracing. Making the client configurable now avoids touching
delivery logic at observability time.

## What to build

### `internal/shared/httpclient/client.go`

```
Option func(*options)

WithTimeout(d time.Duration) Option
WithTransport(t http.RoundTripper) Option   ← used by observability task for otelhttp

New(opts ...Option) *http.Client
  — default timeout: 10s
  — default transport: http.DefaultTransport
  — applies each option in order
```

No logging, no business logic — just a configured `*http.Client`. Logging stays in the caller
(`slog.*Context` at call site). OTel transport injected at observability time via `WithTransport`.

## Change to delivery client

```go
// before
http: &http.Client{Timeout: timeout}

// after
http: httpclient.New(httpclient.WithTimeout(timeout))
```

Constructor signature stays `NewClient(webhookURL string, timeout time.Duration) Client` —
no breaking change. The transport swap at observability time happens in processor `main()`:

```go
client := delivery.NewClient(url, timeout,
    delivery.WithTransport(otelhttp.NewTransport(http.DefaultTransport)),
)
```

Which requires adding `WithTransport` as an option to `NewClient` as well.

## Tests

`internal/shared/httpclient/client_test.go`:
- `TestNew_defaultTimeout` — default client has 10s timeout
- `TestNew_withTimeout` — WithTimeout overrides default
- `TestNew_withTransport` — custom transport is used
