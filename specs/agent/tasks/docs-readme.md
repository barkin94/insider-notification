# docs-readme

**Specs:** `system/ARCHITECTURE.md`, `api-service/API_CONTRACT.md`
**Verification:** `system/VERIFICATION.md` § Docs
**Status:** pending

## What to build

### swag annotations

Add `// @...` swag annotations to all handlers in `api/internal/handler/`:
- `@Summary`, `@Description`, `@Tags`, `@Accept`, `@Produce`
- `@Param`, `@Success`, `@Failure` for every response code in `API_CONTRACT.md`
- `@Router` for each endpoint

Run `swag init -dir api -output docs` → generates `docs/swagger.json`, `docs/swagger.yaml`, `docs/docs.go`.
Mount Swagger UI at `/swagger` in the chi router.

### `README.md`

Sections:
1. **Overview** — what the system does, two-sentence summary
2. **Architecture** — copy the ASCII component diagram from `ARCHITECTURE.md`
3. **Quick Start** — `cp .env.example .env` → fill WEBHOOK_URL → `docker-compose up`
4. **API Examples** — curl examples for POST /notifications, GET /notifications/:id, POST /notifications/batch, POST /notifications/:id/cancel
5. **Design Decisions** — brief bullets for each ADR from `ARCHITECTURE.md`
6. **Running Tests** — `go test ./...` and `go test -race ./...`

## Tests

- `swag init -dir api` exits 0
- `GET /swagger/index.html` returns 200 after docker-compose up
