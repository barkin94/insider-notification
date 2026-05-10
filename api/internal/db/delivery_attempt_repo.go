package db

import (
	"context"

	"github.com/barkin/insider-notification/shared/model"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type bunDeliveryAttemptRepo struct{ db *bun.DB }

func NewDeliveryAttemptRepository(db *bun.DB) DeliveryAttemptRepository {
	return &bunDeliveryAttemptRepo{db: db}
}

func (r *bunDeliveryAttemptRepo) Create(ctx context.Context, a *model.DeliveryAttempt) error {
	_, err := r.db.NewInsert().Model(a).
		On("CONFLICT (notification_id, attempt_number) DO NOTHING").
		Exec(ctx)
	return err
}

func (r *bunDeliveryAttemptRepo) ListByNotificationID(ctx context.Context, notificationID uuid.UUID) ([]*model.DeliveryAttempt, error) {
	var attempts []*model.DeliveryAttempt
	err := r.db.NewSelect().Model(&attempts).
		Where("notification_id = ?", notificationID).
		OrderExpr("attempt_number ASC").
		Scan(ctx)
	return attempts, err
}
