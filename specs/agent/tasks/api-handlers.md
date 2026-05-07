# api-handlers

**Specs:** `api-service/API_CONTRACT.md`, `api-service/DATA_MODEL.md`
**Verification:** `api-service/VERIFICATION.md` § HTTP Handlers
**Status:** complete

## Architecture

Handler / Service / Repository. Handlers decode and encode only. Service owns all business logic.

```
api/internal/
  handler/
    router.go         — Deps, NewRouter, chi route registration, middleware
    notification.go   — thin HTTP handlers; call service, write response
    health.go         — GET /health (direct DB + Redis ping, no service)
    response.go       — writeJSON, writeError helpers; error response types
  service/
    notification.go   — NotificationService interface + implementation
```

## What to build

### `api/internal/service/notification.go`

```
NotificationService interface:
  Create(ctx, req CreateRequest) (*model.Notification, error)
  GetByID(ctx, id uuid.UUID) (*model.Notification, []*model.DeliveryAttempt, error)
  List(ctx, filter db.ListFilter) ([]*model.Notification, int, error)
  Cancel(ctx, id uuid.UUID) (*model.Notification, error)
  CreateBatch(ctx, reqs []CreateRequest) (uuid.UUID, []BatchResult, error)

CreateRequest struct:
  Recipient  string
  Channel    string
  Content    string
  Priority   string          // defaults to "normal" if empty
  Metadata   json.RawMessage // optional
  DeliverAfter *time.Time   // optional

BatchResult struct:
  Index  int
  Status string   // "accepted" | "rejected"
  ID     *uuid.UUID
  Error  *string

notificationService struct:
  repo      db.NotificationRepository
  attempts  db.DeliveryAttemptRepository
  publisher StreamPublisher

StreamPublisher interface (defined in service package):
  Publish(ctx context.Context, topic string, payload any) error

Create:
  1. Build model.Notification (uuid.New(), status=pending, MaxAttempts=4)
  2. repo.Create
  3. Build NotificationCreatedEvent from full notification payload
  4. publisher.Publish to topic matching priority (TopicHigh/Normal/Low)
  5. Return notification

Cancel:
  — repo.Transition(id, "pending", "cancelled")
  — db.ErrTransitionFailed → return as-is; handler maps to 409

CreateBatch:
  — generate one batch_id for all valid items
  — validate each item independently (same rules as Create)
  — call Create for each valid item (with batch_id set on the notification)
  — collect per-item accepted/rejected results
```

### `api/internal/handler/router.go`

```
Deps struct:
  Service NotificationService
  Logger  *slog.Logger
  DB      *pgxpool.Pool   // health check only
  Redis   *redis.Client   // health check only

NewRouter(deps Deps) http.Handler
  — mounts all routes under /api/v1
  — attaches Logger middleware
```

### `api/internal/handler/notification.go`

```
POST /notifications             → CreateNotification
GET  /notifications             → ListNotifications
GET  /notifications/{id}        → GetNotification
POST /notifications/{id}/cancel → CancelNotification
POST /notifications/batch       → CreateBatch
```

### `api/internal/handler/health.go`

```
GET /health → HealthCheck
  — SELECT 1 on PostgreSQL (2s timeout)
  — PING on Redis (1s timeout)
  — 200 if both pass, 503 if either fails
```

### `api/internal/handler/response.go`

```
writeJSON(w, status int, body any)
writeError(w, status int, code, message string, details any)

ErrorResponse struct{ Error ErrorBody }
ErrorBody struct{ Code, Message string; Details any }
```

## Validation rules (enforced in service.Create / service.CreateBatch)

| Field | Rule |
|-------|------|
| recipient | required, max 255 chars |
| channel | required, one of: sms, email, push |
| content | required; max 1600 (sms), 100000 (email), 4096 (push) |
| priority | optional, default normal; one of: high, normal, low |
| batch size | max 1000 items (checked in handler before calling service) |

## Tests

### `api/internal/service/notification_test.go` (mock repos + publisher)

- `TestCreate_success` — valid request → notification created + event published to correct topic
- `TestCreate_invalidChannel` — bad channel → validation error returned
- `TestCreate_contentTooLong` — SMS content > 1600 → validation error
- `TestCreate_publishFailure` — publisher returns error → error propagated
- `TestCancel_success` — pending → cancelled
- `TestCancel_transitionFailed` — repo returns ErrTransitionFailed → error propagated
- `TestCreateBatch_mixedResults` — some valid, some invalid → correct accepted/rejected counts

### `api/internal/handler/notification_test.go` (httptest + mock service)

- `TestCreateNotification_201` — valid body → 201 + correct response shape
- `TestCreateNotification_400_missingContent` — missing content → 400 VALIDATION_ERROR
- `TestCreateNotification_400_contentTooLong` — SMS > 1600 → 400
- `TestListNotifications_pagination` — page/page_size in response
- `TestListNotifications_filterByStatus` — status filter applied
- `TestGetNotification_200` — includes delivery_attempts array
- `TestGetNotification_404` — unknown ID → 404 NOT_FOUND
- `TestCancelNotification_200` — pending → cancelled
- `TestCancelNotification_409` — transition failed → 409 INVALID_STATUS_TRANSITION
- `TestCreateBatch_207` — mixed valid/invalid → correct accepted/rejected counts
- `TestCreateBatch_400_tooLarge` — 1001 items → 400

### `api/internal/handler/health_test.go` (httptest, real deps mocked via interfaces)

- `TestHealth_200` — both checks pass
- `TestHealth_503` — one check fails → 503 with error detail
