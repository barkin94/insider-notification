# Insider Notification Service

A notification delivery system built in Go. The API accepts notification requests, publishes them to Redis Streams by priority, the Processor picks them up for delivery via webhook, and publishes delivery results back to Redis Streams.

## Architecture

```
          ┌─────────────┐        Redis Streams         ┌─────────────────┐
 Client ──▶   API        ├──(high / normal / low)──────▶   Processor      │
          │  :8080       │                              │                  │
          │              │◀────(delivery results)───────│  worker pool     │
          └──────┬───────┘                              └────────┬─────────┘
                 │                                               │
            PostgreSQL                                     Webhook target
```

**Services**

| Service | Role |
|---------|------|
| `api` | HTTP API — create, list, cancel notifications |
| `processor` | Consumes streams, delivers via webhook, writes results back |
| `postgres` | Persistent store for notifications and delivery attempts |
| `redis` | Redis Streams for async message passing |
| `otel-collector` | Receives OTLP traces, forwards to Tempo |
| `tempo` | Trace storage, queried by Grafana |
| `prometheus` | Scrapes `/metrics` from OTel Collector |
| `loki` | Logs storage, queried by Grafana
| `grafana` | Dashboards — metrics (Prometheus) + traces (Tempo) + logs (Loki) |

## Prerequisites

- Docker and Docker Compose

Run `make help` to see all available commands.

## Running the system

### 1. Configure environment

Before running the services, `api/` and `processor/` folders need `.env` files inside. Each contains `.env.example` files that are pre-filled for docker-compose, which you can simply create a copy of and then rename to `.env`.


### 2. Start all services

```bash
make up
```

Migrations run automatically as part of `make up` — dedicated `migrate-api` and `migrate-processor` containers run first and exit before the services start.

### 3. Verify

```bash
curl http://localhost:8080/api/v1/health
```

Expected: `{"status":"ok"}`

## API

Base URL: `http://localhost:8080`

**Swagger UI:** `http://localhost:8080/api/v1/docs/index.html`

## Observability

| URL | What you see |
|-----|-------------|
| `http://localhost:3000` | Grafana — metrics dashboard + traces + logs |
| `http://localhost:9090` | Prometheus — raw metrics |
| `http://localhost:3200` | Tempo — raw trace API |

Both services expose `/metrics` on their HTTP ports (`:8080` and `:8081`).

## Development

**Run tests**

```bash
make test
```

Integration tests spin up Postgres and Redis via testcontainers — Docker must be running.

**Lint**

```bash
make lint
```

**Regenerate Swagger docs** (only needed after changing handler signatures or adding endpoints)

```bash
make swag
```

**Run locally** (requires Postgres and Redis on localhost)

Start just the infrastructure, then run each service directly:

```bash
make infra

# in separate terminals
go run ./api/cmd
go run ./processor/cmd
```

Change `REDIS_ADDR` and `DATABASE_URL` in the `.env` files to `localhost` for local runs. Migrations can be applied via the migrate container: `docker compose run --rm migrate-api`.

## Functional Requirements Status

### Notification Management API

- [x] Create notification requests with recipient, channel, content, and priority
- [x] Support batch creation (up to 1000 notifications per request)
- [x] Query notification status by ID or batch ID
- [x] Cancel pending notifications
- [x] List notifications with filtering (status, channel, date range) and pagination

### Processing Engine

- [x] Process notifications asynchronously via queue workers
- [x] Implement rate limiting: maximum 100 messages per second per channel
- [x] Priority queue support (high, normal, low)
- [x] Content validation (character limits, required fields)
- [x] Idempotency support to prevent duplicate sends

### Delivery & Retry Logic

- [x] Will require thinking and design by candidate

### Observability

- [x] Real-time metrics endpoint (OpenTelemetry exports metrics to Prometheus)
- [x] Structured logging with correlation IDs (Correlation is achieved with OpenTelemetry)
- [x] Health check endpoint

### Bonus Features

- [x] Failure Handling
- [x] Scheduled Notifications: Allow scheduling notifications for future delivery
- [ ] Template System: Support message templates with variable substitution
- [ ] WebSocket Updates: Real-time status updates via WebSocket
- [x] Distributed Tracing
- [ ] GitHub Actions CI/CD: Automated testing and linting pipeline