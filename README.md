# Insider Notification Service

A notification delivery system built in Go. The API accepts notification requests, publishes them to Redis Streams by priority, and the Processor picks them up for delivery via webhook.

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
| `prometheus` | Scrapes `/metrics` from both services |
| `grafana` | Dashboards — metrics (Prometheus) + traces (Tempo) |

## Prerequisites

- Docker and Docker Compose

Run `make help` to see all available commands.

## Running the system

### 1. Configure environment

The `.env` files in `api/` and `processor/` are pre-filled for docker-compose. The one value you must set before starting:

```
# processor/.env
WEBHOOK_URL=https://webhook.site/<your-uuid>   # replace with a real webhook.site URL
```

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

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Health check (Postgres + Redis) |
| `POST` | `/api/v1/notifications` | Create a notification |
| `GET` | `/api/v1/notifications` | List notifications |
| `POST` | `/api/v1/notifications/batch` | Create a batch of notifications |
| `GET` | `/api/v1/notifications/{id}` | Get a notification by ID |
| `POST` | `/api/v1/notifications/{id}/cancel` | Cancel a pending notification |

**Create notification example**

```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "user@example.com",
    "channel": "email",
    "content": "Hello!",
    "priority": "normal"
  }'
```

## Observability

| URL | What you see |
|-----|-------------|
| `http://localhost:3000` | Grafana (admin / admin) — metrics dashboard + traces |
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

**Run locally** (requires Postgres and Redis on localhost)

Start just the infrastructure, then run each service directly:

```bash
make infra

# in separate terminals
go run ./api/cmd
go run ./processor/cmd
```

Change `REDIS_ADDR` and `DATABASE_URL` in the `.env` files to `localhost` for local runs. Migrations can be applied via the migrate container: `docker compose run --rm migrate-api`.
