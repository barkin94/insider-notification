# delivery-client

**Specs:** `processor-service/RETRY_POLICY.md`, `system/OBSERVABILITY.md`
**Verification:** `processor-service/VERIFICATION.md` § Delivery Client
**Status:** pending

## What to build

### `internal/processor/delivery/client.go`
```
Result struct:
  Success        bool
  Retryable      bool
  StatusCode     int
  LatencyMS      int64
  ProviderMsgID  string
  ErrorMessage   string

Client interface:
  Send(ctx context.Context, n *model.Notification, correlationID string) (Result, error)

httpClient struct{ http *http.Client; webhookURL string }

Send(ctx, n, correlationID):
  — POST webhookURL with JSON body: {id, channel, recipient, content}
  — header: X-Correlation-ID: correlationID
  — measure latency from request start to response
  — 202 → Result{Success: true}
  — 400 / 401 / 403 → Result{Retryable: false}
  — 5xx / 429 / timeout / network error → Result{Retryable: true}
  — any other non-202 → Result{Retryable: true}
```

## Tests

`internal/processor/delivery/client_test.go` using `httptest.NewServer`:

- `TestSend_202_success` — mock returns 202 → Result.Success=true, Retryable=false
- `TestSend_400_nonRetryable` — mock returns 400 → Retryable=false
- `TestSend_401_nonRetryable` — mock returns 401 → Retryable=false
- `TestSend_403_nonRetryable` — mock returns 403 → Retryable=false
- `TestSend_503_retryable` — mock returns 503 → Retryable=true
- `TestSend_429_retryable` — mock returns 429 → Retryable=true
- `TestSend_timeout_retryable` — server hangs past client timeout → Retryable=true
- `TestSend_correlationHeader` — X-Correlation-ID forwarded to mock server
- `TestSend_latency_measured` — LatencyMS > 0 on any response
