package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID           uuid.UUID       `bun:",pk" db:"id"`
	BatchID      *uuid.UUID      `db:"batch_id"`
	Recipient    string          `db:"recipient"`
	Channel      string          `db:"channel"`
	Content      string          `db:"content"`
	Priority     string          `db:"priority"`
	Status       string          `db:"status"`
	DeliverAfter *time.Time      `db:"deliver_after"`
	Attempts     int             `db:"attempts"`
	MaxAttempts  int             `db:"max_attempts"`
	Metadata     json.RawMessage `db:"metadata"`
	CreatedAt    time.Time       `db:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at"`
}

type DeliveryAttempt struct {
	ID               uuid.UUID       `bun:",pk" db:"id"`
	NotificationID   uuid.UUID       `db:"notification_id"`
	AttemptNumber    int             `db:"attempt_number"`
	Status           string          `db:"status"`
	HTTPStatusCode   *int            `db:"http_status_code"`
	ProviderResponse json.RawMessage `db:"provider_response"`
	ErrorMessage     *string         `db:"error_message"`
	LatencyMS        *int            `db:"latency_ms"`
	AttemptedAt      time.Time       `db:"attempted_at"`
}
