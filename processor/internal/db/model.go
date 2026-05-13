package db

import (
	"time"

	shareddb "github.com/barkin/insider-notification/shared/db"
	"github.com/google/uuid"
)

type DeliveryAttempt struct {
	shareddb.BaseModel `bun:"table:processor.delivery_attempts"`
	NotificationID     uuid.UUID  `bun:"notification_id"`
	AttemptNumber      int        `bun:"attempt_number"`
	Status             string     `bun:"status"`
	Priority           string     `bun:"priority"`
	RetryAfter         *time.Time `bun:"retry_after"`
}
