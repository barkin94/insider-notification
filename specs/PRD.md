# PRD — Notification System

## Goal

Design and implement a scalable notification system that processes and delivers messages
through multiple channels (SMS, Email, Push). The system must handle high throughput,
ensure reliable delivery, and provide real-time status tracking.

## Business Context

The company sends millions of notifications daily. The system must handle burst traffic
(flash sales, breaking news), retry failed deliveries intelligently, and provide delivery
visibility for both internal teams and API consumers.

## In Scope

- Notification Management REST API (CRUD, batch, filtering, pagination)
- Async processing engine with priority queues and rate limiting
- Delivery & retry logic with exponential backoff
- Idempotency support
- Observability: metrics, structured logging, health checks
- Scheduled notifications (future delivery)
- WebSocket real-time status updates
- Message template system with variable substitution
- GitHub Actions CI/CD pipeline
- External provider integration via webhook.site
- Docker Compose one-command setup
- OpenAPI/Swagger documentation
- Versioned DB migrations
- Full test suite

## Out of Scope

- Authentication / API key management (not required by case study)
- Multi-tenant rate limiting (rate limits are global per channel)
- Real SMS / Email / Push provider integration (webhook.site mock only)
- Horizontal auto-scaling infrastructure (Docker Compose scope only)
- Admin UI

## Success Criteria

- Single notification and batch (up to 1000) creation works correctly
- Notifications are processed asynchronously via priority queues
- Rate limiting enforces 100 msg/s per channel
- Failed deliveries are retried with exponential backoff + jitter (max 4 attempts)
- Duplicate notifications are rejected via idempotency checks
- Notification status is queryable by ID and batch ID
- Pending notifications can be cancelled
- Metrics endpoint exposes queue depth, success/failure rates, latency
- WebSocket clients receive real-time status updates
- Scheduled notifications are delivered at the correct time
- Templates resolve variables correctly before delivery
- `docker-compose up` starts the full system
- All tests pass with a single command

## Constraints

- Language: Go 1.2x
- External provider: webhook.site (simulated, returns 202 Accepted)
- DB: MongoDB 7 (replica set)
- Cache / Rate limiter / Queue broker: Redis
