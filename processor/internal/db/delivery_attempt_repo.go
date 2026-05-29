package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type bunDeliveryAttemptRepo struct{ db *bun.DB }

func NewDeliveryAttemptRepository(db *bun.DB) DeliveryAttemptRepository {
	return &bunDeliveryAttemptRepo{db: db}
}

func (r *bunDeliveryAttemptRepo) Create(ctx context.Context, a *DeliveryAttempt) error {
	_, err := r.db.NewInsert().Model(a).
		On("CONFLICT (notification_id) DO UPDATE SET attempt_number = EXCLUDED.attempt_number, retry_after = EXCLUDED.retry_after, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// CountByNotificationID returns the attempt_number stored on the single delivery
// attempt entity for the notification, or 0 if none exists yet.
func (r *bunDeliveryAttemptRepo) CountByNotificationID(ctx context.Context, id uuid.UUID) (int, error) {
	var n int
	err := r.db.NewSelect().
		TableExpr("delivery_attempts").
		ColumnExpr("attempt_number").
		Where("notification_id = ?", id).
		Scan(ctx, &n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

func (r *bunDeliveryAttemptRepo) FindDueRetries(ctx context.Context) ([]*DeliveryAttempt, error) {
	var rows []*DeliveryAttempt
	err := r.db.NewSelect().
		Model(&rows).
		Where("retry_after IS NOT NULL").
		Where("retry_after <= NOW()").
		Scan(ctx)
	return rows, err
}
