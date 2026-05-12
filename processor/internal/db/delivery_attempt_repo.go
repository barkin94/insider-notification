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
