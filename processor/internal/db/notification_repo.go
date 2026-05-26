package db

import (
	"context"

	"github.com/barkin/insider-notification/shared/model"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// NotificationRow holds the fields the scheduler needs from public.notifications.
type NotificationRow struct {
	ID          uuid.UUID `bun:"id"`
	Priority    string    `bun:"priority"`
	Channel     string    `bun:"channel"`
	Recipient   string    `bun:"recipient"`
	Content     string    `bun:"content"`
	MaxAttempts int       `bun:"max_attempts"`
}

type bunNotificationReader struct{ db *bun.DB }

func NewNotificationReader(db *bun.DB) NotificationReader {
	return &bunNotificationReader{db: db}
}

func (r *bunNotificationReader) FindScheduledDue(ctx context.Context) ([]NotificationRow, error) {
	var rows []NotificationRow
	err := r.db.NewSelect().
		TableExpr("notifications").
		ColumnExpr("id, priority, channel, recipient, content, max_attempts").
		Where("deliver_after IS NOT NULL").
		Where("deliver_after <= NOW()").
		Where("status = ?", model.StatusPending).
		OrderExpr("deliver_after ASC").
		Limit(500).
		Scan(ctx, &rows)
	return rows, err
}

func (r *bunNotificationReader) FindByIDs(ctx context.Context, ids []uuid.UUID) ([]NotificationRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []NotificationRow
	err := r.db.NewSelect().
		TableExpr("notifications").
		ColumnExpr("id, priority, channel, recipient, content, max_attempts").
		Where("id IN (?)", bun.List(ids)).
		Scan(ctx, &rows)
	return rows, err
}
