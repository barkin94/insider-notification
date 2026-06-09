package db

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type DeliveryAttempt struct {
	bun.BaseModel `bun:"table:delivery_attempts"`

	ID             uuid.UUID  `bun:"id,pk,type:uuid"`
	NotificationID string     `bun:"notification_id,type:uuid,notnull,unique"`
	AttemptNumber  int        `bun:"attempt_number,notnull"`
	RetryAfter     *time.Time `bun:"retry_after"`
	Priority       string     `bun:"priority"`
	Channel        string     `bun:"channel,notnull"`
	Recipient      string     `bun:"recipient,notnull"`
	Content        string     `bun:"content,notnull"`
	MaxAttempts    int        `bun:"max_attempts,notnull"`
}
