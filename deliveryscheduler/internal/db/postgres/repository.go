package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	schedulerdb "github.com/barkin94/insider-notification/deliveryscheduler/internal/db"
)

type pgScheduledNotificationRepo struct{ db *bun.DB }

var _ schedulerdb.ScheduledNotificationRepository = (*pgScheduledNotificationRepo)(nil)

func NewScheduledNotificationRepository(db *bun.DB) schedulerdb.ScheduledNotificationRepository {
	return &pgScheduledNotificationRepo{db: db}
}

func (r *pgScheduledNotificationRepo) UpsertAll(ctx context.Context, notifications []*schedulerdb.ScheduledNotification) error {
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

func (r *pgScheduledNotificationRepo) DeleteByNotificationID(ctx context.Context, notificationID string) error {
	_, err := r.db.NewDelete().
		Model((*schedulerdb.ScheduledNotification)(nil)).
		Where("notification_id = ?", notificationID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("pg delete scheduled notification: %w", err)
	}
	return nil
}

func (r *pgScheduledNotificationRepo) DeleteByScheduledAtBeforeReturning(ctx context.Context, before time.Time, limit int) ([]*schedulerdb.ScheduledNotification, error) {
	if limit < 1 {
		limit = 1
	}
	var notifications []*schedulerdb.ScheduledNotification
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
