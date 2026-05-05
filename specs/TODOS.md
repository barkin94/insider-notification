# TODOS

Features deferred from the initial implementation. Each entry includes enough
context to write a spec and implement when prioritized.

---

## WebSocket: Real-Time Status Updates

**What:** A WebSocket endpoint that allows clients to subscribe to live status
changes for a specific notification.

**Endpoint:** `WS /ws/status/:notification_id`

**Behavior:**
- On connect: send current notification status immediately
- On each status change: broadcast new status to all subscribers of that notification ID
- Close connection automatically on terminal status (`delivered`, `failed`, `cancelled`)
- Ping/pong keepalive every 30 seconds

**Server → Client message shape:**
```json
{
  "notification_id": "uuid",
  "status": "delivered",
  "updated_at": "ISO8601",
  "attempt_number": 1
}
```

**Design notes:**
- In-process hub with per-notification-ID subscription rooms
- Status changes published to hub immediately after each DB write in the delivery worker
- Not horizontally scalable without a Redis pub/sub adapter (acceptable for Docker Compose scope)
- Requires `gorilla/websocket` dependency

**Spec files to write when prioritized:**
- Add `WS /ws/status/:notification_id` to `API_CONTRACT.md`
- Add WebSocket Hub ADR to `ARCHITECTURE.md`
- Add `internal/api/ws/` to project layout in `ARCHITECTURE.md`
- Add WebSocket section to `VERIFICATION.md`
