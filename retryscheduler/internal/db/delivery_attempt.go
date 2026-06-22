package db

import (
	"time"

	"github.com/uptrace/bun"
)

type DeliveryAttempt struct {
	bun.BaseModel `bun:"table:delivery_attempts"`

	NotificationID string     `bun:"notification_id,pk,type:uuid,notnull"`
	AttemptNumber  int        `bun:"attempt_number,notnull"`
	RetryAfter     *time.Time `bun:"retry_after"`
	Priority       string     `bun:"priority"`
	Channel        string     `bun:"channel,notnull"`
	Recipient      string     `bun:"recipient,notnull"`
	Content        string     `bun:"content,notnull"`
	MaxAttempts    int        `bun:"max_attempts,notnull"`
}
