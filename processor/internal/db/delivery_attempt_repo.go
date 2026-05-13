package db

import (
	"context"

	"github.com/uptrace/bun"
)

type bunDeliveryAttemptRepo struct{ db *bun.DB }

func NewDeliveryAttemptRepository(db *bun.DB) DeliveryAttemptRepository {
	return &bunDeliveryAttemptRepo{db: db}
}

func (r *bunDeliveryAttemptRepo) Create(ctx context.Context, a *DeliveryAttempt) error {
	_, err := r.db.NewInsert().Model(a).
		On("CONFLICT (notification_id, attempt_number) DO NOTHING").
		Exec(ctx)
	return err
}

func (r *bunDeliveryAttemptRepo) FindDueRetries(ctx context.Context) ([]*DeliveryAttempt, error) {
	var rows []*DeliveryAttempt
	err := r.db.NewSelect().
		Model(&rows).
		DistinctOn("(notification_id)").
		Where("status = ?", "failed").
		Where("retry_after IS NOT NULL").
		Where("retry_after <= NOW()").
		OrderExpr("notification_id, attempt_number DESC").
		Scan(ctx)
	return rows, err
}
