package db

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

type ScheduledNotificationRepository interface {
	// UpsertAll upserts all rows in a single round trip.
	UpsertAll(ctx context.Context, notifications []*ScheduledNotification) error
	// DeleteByScheduledAtBeforeReturning atomically claims and removes scheduled notifications
	// whose scheduled_at is at or before the given time, up to limit entries.
	DeleteByScheduledAtBeforeReturning(ctx context.Context, before time.Time, limit int) ([]*ScheduledNotification, error)
}

type pgScheduledNotificationRepo struct{ db *bun.DB }

var _ ScheduledNotificationRepository = (*pgScheduledNotificationRepo)(nil)

func NewScheduledNotificationRepository(db *bun.DB) ScheduledNotificationRepository {
	return &pgScheduledNotificationRepo{db: db}
}

func (r *pgScheduledNotificationRepo) UpsertAll(ctx context.Context, notifications []*ScheduledNotification) error {
	if len(notifications) == 0 {
		return nil
	}
	_, err := r.db.NewInsert().
		Model(&notifications).
		On("CONFLICT (notification_id) DO UPDATE").
		Set("scheduled_at = EXCLUDED.scheduled_at").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("pg batch upsert scheduled notifications: %w", err)
	}
	return nil
}

func (r *pgScheduledNotificationRepo) DeleteByScheduledAtBeforeReturning(ctx context.Context, before time.Time, limit int) ([]*ScheduledNotification, error) {
	if limit < 1 {
		limit = 1
	}
	var notifications []*ScheduledNotification
	err := r.db.NewRaw(`
		DELETE FROM scheduled_notifications
		WHERE notification_id IN (
			SELECT notification_id FROM scheduled_notifications
			WHERE scheduled_at IS NOT NULL AND scheduled_at <= ?
			ORDER BY scheduled_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *
		`,
		before, limit,
	).Scan(ctx, &notifications)
	if err != nil {
		return nil, fmt.Errorf("pg find and delete due scheduled notifications: %w", err)
	}
	return notifications, nil
}
