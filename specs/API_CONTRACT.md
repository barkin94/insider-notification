# API CONTRACT — Notification System

## Base URL
`http://localhost:8080/api/v1`

## Common Headers
```
Content-Type: application/json
Idempotency-Key: <client-supplied string, optional>   ← for POST /notifications only
```

## Common Error Response
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable description",
    "details": {}         // optional field-level errors
  }
}
```

## Error Codes
| HTTP Status | Code | Meaning |
|-------------|------|---------|
| 400 | `VALIDATION_ERROR` | Invalid request body or field constraints |
| 404 | `NOT_FOUND` | Resource does not exist |
| 409 | `DUPLICATE_NOTIFICATION` | Idempotency key already used |
| 409 | `INVALID_STATUS_TRANSITION` | Cannot cancel a delivered/failed notification |
| 429 | `RATE_LIMITED` | API-level rate limit exceeded (not channel rate limit) |
| 500 | `INTERNAL_ERROR` | Unhandled server error |

---

## Endpoints

---

### POST /notifications
Create a single notification.

**Request:**
```json
{
  "recipient": "+905551234567",        // required, string, max 255
  "channel": "sms",                   // required, enum: sms | email | push
  "content": "Your OTP is 123456",    // required unless template_id provided, string
  "priority": "high",                 // optional, enum: high | normal | low, default: normal
  "metadata": {}                      // optional, arbitrary JSON object
}
```

**Rules:**
- `content` is required
- Content length validated per channel (see DATA_MODEL.md)

**Response: 201 Created**
```json
{
  "id": "uuid",
  "status": "pending",
  "channel": "sms",
  "recipient": "+905551234567",
  "priority": "normal",
  "created_at": "2024-06-01T09:00:00Z"
}
```

**Response: 409 Conflict** (duplicate idempotency key)
```json
{
  "error": {
    "code": "DUPLICATE_NOTIFICATION",
    "message": "Notification already exists for this idempotency key",
    "details": {
      "existing_id": "uuid-of-original-notification"
    }
  }
}
```

---

### POST /notifications/batch
Create up to 1000 notifications in a single request.

**Request:**
```json
{
  "notifications": [
    {
      "recipient": "+905551234567",
      "channel": "sms",
      "content": "Flash sale starts now!",
      "priority": "high"
    }
    // ... up to 1000 items
  ]
}
```

**Rules:**
- Max 1000 notifications per request (400 if exceeded)
- Each item validated independently
- All valid items are created even if some fail validation
- A `batch_id` UUID is generated and assigned to all successfully created notifications

**Response: 207 Multi-Status**
```json
{
  "batch_id": "uuid",
  "total": 1000,
  "accepted": 998,
  "rejected": 2,
  "results": [
    { "index": 0, "status": "accepted", "id": "uuid" },
    { "index": 1, "status": "accepted", "id": "uuid" },
    { "index": 5, "status": "rejected", "error": { "code": "VALIDATION_ERROR", "message": "content exceeds SMS limit" } }
    // only rejected items included in results array to keep payload small
  ]
}
```

---

### GET /notifications/:id
Get a single notification by ID.

**Response: 200 OK**
```json
{
  "id": "uuid",
  "batch_id": "uuid | null",
  "recipient": "+905551234567",
  "channel": "sms",
  "content": "Your OTP is 123456",
  "priority": "normal",
  "status": "delivered",
  "provider_message_id": "uuid-from-provider",
  "attempts": 1,
  "max_attempts": 4,
  "metadata": null,
  "created_at": "ISO8601",
  "updated_at": "ISO8601",
  "delivery_attempts": [
    {
      "id": "uuid",
      "attempt_number": 1,
      "status": "success",
      "http_status_code": 202,
      "latency_ms": 143,
      "attempted_at": "ISO8601"
    }
  ]
}
```

---

### GET /notifications
List notifications with filtering and pagination.

**Query Parameters:**
```
status       string    Filter by status (pending|processing|delivered|failed|cancelled)
channel      string    Filter by channel (sms|email|push)
batch_id     uuid      Filter by batch ID
date_from    ISO8601   Filter created_at >= date_from
date_to      ISO8601   Filter created_at <= date_to
page         int       Page number, default: 1
page_size    int       Items per page, default: 20, max: 100
sort         string    Field to sort by: created_at | updated_at, default: created_at
order        string    asc | desc, default: desc
```

**Response: 200 OK**
```json
{
  "data": [ /* array of notification objects (same shape as GET /notifications/:id, without delivery_attempts) */ ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 4821,
    "total_pages": 242
  }
}
```

---

### POST /notifications/:id/cancel
Cancel a pending or scheduled notification.

**Rules:**
- Only `pending` notifications can be cancelled
- Returns 409 if status is `processing`, `delivered`, `failed`, or `cancelled`

**Request:** empty body

**Response: 200 OK**
```json
{
  "id": "uuid",
  "status": "cancelled",
  "updated_at": "ISO8601"
}
```

---

### GET /metrics
Real-time system metrics.

**Response: 200 OK**
```json
{
  "queues": {
    "high":   { "depth": 142 },
    "normal": { "depth": 1893 },
    "low":    { "depth": 44 }
  },
  "delivery": {
    "sms": {
      "sent": 48291,
      "failed": 103,
      "success_rate": 0.9979,
      "avg_latency_ms": 187
    },
    "email": {
      "sent": 12004,
      "failed": 22,
      "success_rate": 0.9982,
      "avg_latency_ms": 211
    },
    "push": {
      "sent": 9841,
      "failed": 55,
      "success_rate": 0.9944,
      "avg_latency_ms": 134
    }
  },
  "rate_limiter": {
    "sms":   { "available_tokens": 87, "capacity": 100 },
    "email": { "available_tokens": 100, "capacity": 100 },
    "push":  { "available_tokens": 62, "capacity": 100 }
  },
  "uptime_seconds": 38291
}
```

---

### GET /health
Health check endpoint.

**Response: 200 OK**
```json
{
  "status": "ok",
  "checks": {
    "mongodb": "ok",
    "redis": "ok"
  },
  "version": "1.0.0"
}
```

**Response: 503 Service Unavailable** (if any check fails)
```json
{
  "status": "degraded",
  "checks": {
    "mongodb": "ok",
    "redis": "error: connection refused"
  }
}
```

