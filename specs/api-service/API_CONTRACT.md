# API CONTRACT — Notification System

## Base URL
`http://localhost:8080/api/v1`

## Common Headers
```
Content-Type: application/json
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
  "content": "Your OTP is 123456",    // required, string
  "priority": "high",                 // optional, enum: high | normal | low, default: normal
  "metadata": {},                     // optional, arbitrary JSON object
  "deliver_after": "2024-06-01T10:00:00Z" // optional, RFC3339; notification is enqueued immediately but worker skips until this time passes
}
```

**Rules:**
- `content` is required
- Content length validated per channel (see DATA_MODEL.md)
- `deliver_after` must be a valid RFC3339 timestamp if provided

**Response: 201 Created**
```json
{
  "id": "uuid",
  "status": "pending",
  "channel": "sms",
  "recipient": "+905551234567",
  "priority": "normal",
  "deliver_after": "2024-06-01T10:00:00Z",
  "created_at": "2024-06-01T09:00:00Z"
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
  "attempts": 1,
  "max_attempts": 4,
  "deliver_after": "ISO8601 | null",
  "metadata": null,
  "created_at": "ISO8601",
  "updated_at": "ISO8601"
}
```

---

### GET /notifications
List notifications with filtering and pagination.

**Query Parameters:**
```
status       string    Filter by status (pending|delivered|failed|cancelled)
channel      string    Filter by channel (sms|email|push)
batch_id     uuid      Filter by batch ID
date_from    ISO8601   Filter created_at >= date_from
date_to      ISO8601   Filter created_at <= date_to
page_size    int       Items per page, default: 20, max: 100
cursor       string    Opaque cursor (base64url-encoded UUID v7). When present, returns the page after this ID.
```

Results are always ordered by `id DESC` (newest first). `cursor` and offset `page` are mutually exclusive — `cursor` takes precedence.

**Response: 200 OK**
```json
{
  "data": [ /* array of notification objects (same shape as GET /notifications/:id) */ ],
  "pagination": {
    "page_size": 20,
    "total": 4821,
    "next_cursor": "eyJ1dWlkIjoiMDFIWC4uLiJ9"
  }
}
```

`next_cursor` is `null` on the last page.

---

### POST /notifications/:id/cancel
Cancel a pending notification.

**Rules:**
- Only `pending` notifications can be cancelled
- Returns 409 if status is `delivered`, `failed`, or `cancelled`

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

### GET /health
Health check endpoint.

**Response: 200 OK**
```json
{
  "status": "ok",
  "checks": {
    "postgresql": "ok",
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
    "postgresql": "ok",
    "redis": "error: connection refused"
  }
}
```

