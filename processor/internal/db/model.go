package db

import (
	"time"

	"github.com/google/uuid"

	shareddb "github.com/barkin/insider-notification/shared/db"
)

type DeliveryAttempt struct {
	shareddb.BaseModel `bun:"table:delivery_attempts"`
	NotificationID     uuid.UUID  `bun:"notification_id"`
	AttemptNumber      int        `bun:"attempt_number"`
	Priority           string     `bun:"priority"`
	RetryAfter         *time.Time `bun:"retry_after"`
	Channel            string     `bun:"channel"`
	Recipient          string     `bun:"recipient"`
	Content            string     `bun:"content"`
	MaxAttempts        int        `bun:"max_attempts"`
}
