package db

import (
	"time"

	"github.com/uptrace/bun"
)

type ScheduledNotification struct {
	bun.BaseModel `bun:"table:scheduled_notifications"`

	NotificationID string     `bun:"notification_id,pk,type:uuid,notnull"`
	ScheduledAt    *time.Time `bun:"scheduled_at,notnull"`
}
