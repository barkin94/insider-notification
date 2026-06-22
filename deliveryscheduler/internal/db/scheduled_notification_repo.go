package db

import (
	"context"
	"time"
)

type ScheduledNotificationRepository interface {
	// UpsertAll upserts all rows in a single round trip.
	UpsertAll(ctx context.Context, notifications []*ScheduledNotification) error
	// DeleteByScheduledAtBeforeReturning atomically claims and removes scheduled notifications
	// whose scheduled_at is at or before the given time, up to limit entries.
	DeleteByScheduledAtBeforeReturning(ctx context.Context, before time.Time, limit int) ([]*ScheduledNotification, error)
	// DeleteByNotificationID removes the scheduled notification with the given ID, if it exists.
	DeleteByNotificationID(ctx context.Context, notificationID string) error
}
