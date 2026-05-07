# api-handlers

**Specs:** `api-service/API_CONTRACT.md`, `api-service/DATA_MODEL.md`
**Verification:** `api-service/VERIFICATION.md` § HTTP Handlers
**Status:** pending

## What to build

### `api/internal/handler/router.go`
```
NewRouter(deps Deps) http.Handler
  — mounts all routes under /api/v1
  — attaches CorrelationID and Logger middleware

Deps struct:
  NotificationRepo  db.NotificationRepository
  DeliveryRepo      db.DeliveryAttemptRepository
  IdempotencyRepo   db.IdempotencyRepository
  IdempotencyChecker idempotency.Checker
  Producer          stream.Producer
  Logger            *zap.Logger
  DB                *pgxpool.Pool
  Redis             *redis.Client
```

### `api/internal/handler/notification.go`
```
POST /notifications      → CreateNotification
GET  /notifications      → ListNotifications
GET  /notifications/:id  → GetNotification
POST /notifications/:id/cancel → CancelNotification
```

### `api/internal/handler/batch.go`
```
POST /notifications/batch → CreateBatch
  — validates each item independently
  — inserts all valid items in one transaction with shared batch_id
  — returns 207 with per-item results (only rejected items in results array)
```

### `api/internal/handler/health.go`
```
GET /health → HealthCheck
  — SELECT 1 on PostgreSQL (2s timeout)
  — PING on Redis (1s timeout)
  — 200 if both pass, 503 if either fails
```

## Validation rules (enforced in handlers)

| Field | Rule |
|-------|------|
| recipient | required, max 255 chars |
| channel | required, one of: sms, email, push |
| content | required; max 1600 (sms), 100000 (email), 4096 (push) |
| priority | optional, default normal; one of: high, normal, low |
| batch size | max 1000 items |

## Tests

`api/internal/handler/*_test.go` using `httptest` + mock repositories:

- `TestCreateNotification_201` — valid body → 201 + correct response shape
- `TestCreateNotification_400_missingContent` — missing content → 400 VALIDATION_ERROR
- `TestCreateNotification_400_contentTooLong` — SMS content > 1600 → 400
- `TestCreateNotification_409_duplicateKey` — idempotency hit → 409 DUPLICATE_NOTIFICATION
- `TestListNotifications_pagination` — page/page_size in response
- `TestListNotifications_filterByStatus` — status filter applied
- `TestGetNotification_200` — includes delivery_attempts array
- `TestGetNotification_404` — unknown ID → 404 NOT_FOUND
- `TestCancelNotification_200` — pending → cancelled
- `TestCancelNotification_409` — delivered → 409 INVALID_STATUS_TRANSITION
- `TestCreateBatch_207` — mixed valid/invalid → correct accepted/rejected counts
- `TestCreateBatch_400_tooLarge` — 1001 items → 400
- `TestHealth_200` — both checks pass
- `TestHealth_503` — redis fails → 503 with error detail
