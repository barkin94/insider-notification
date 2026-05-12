package db

import (
	shareddb "github.com/barkin/insider-notification/shared/db"
	"github.com/google/uuid"
)

type DeliveryAttempt struct {
	shareddb.BaseModel `bun:"table:processor.delivery_attempts"`
	NotificationID     uuid.UUID `bun:"notification_id"`
	AttemptNumber      int       `bun:"attempt_number"`
	Status             string    `bun:"status"`
}
