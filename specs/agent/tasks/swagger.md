# swagger

**Status:** pending

## What to build

Serve an OpenAPI 3.0 spec and Swagger UI for the API service.

### Approach

Write a static `api/openapi.yaml` covering all five endpoints. Serve it at two routes:

| Route | What |
|-------|------|
| `GET /api/v1/openapi.yaml` | Raw OpenAPI 3.0 spec |
| `GET /api/v1/docs` | Swagger UI (CDN, no extra packages) |

No code generation, no annotations, no new dependencies.

### Endpoints to document

| Method | Path | Summary |
|--------|------|---------|
| `POST` | `/api/v1/notifications` | Create a notification |
| `GET` | `/api/v1/notifications` | List notifications (with filters + pagination) |
| `POST` | `/api/v1/notifications/batch` | Create a batch of notifications |
| `GET` | `/api/v1/notifications/{id}` | Get a notification with delivery attempts |
| `POST` | `/api/v1/notifications/{id}/cancel` | Cancel a pending notification |
| `GET` | `/api/v1/health` | Health check |

### Request / response shapes

Derived from `api/internal/handler/notification.go` — all types are already defined there.

### Makefile

No changes needed — no code generation step required.
