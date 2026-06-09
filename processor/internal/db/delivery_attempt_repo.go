package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// DeliveryAttemptRepository is the port for delivery attempt persistence and retry scheduling.
type DeliveryAttemptRepository interface {
	// Upsert inserts a delivery attempt row; on conflict updates attempt_number,
	// retry_after, and updated_at so the retry dispatcher picks up the latest state.
	Upsert(ctx context.Context, a *DeliveryAttempt) error
	DeleteByID(ctx context.Context, id string) error
	// FindDueBefore returns attempts whose retry_after is at or before the given time, up to limit entries.
	FindDueBefore(ctx context.Context, before time.Time, limit int) ([]*DeliveryAttempt, error)
}

type pgDeliveryAttemptRepo struct{ db *bun.DB }

var _ DeliveryAttemptRepository = (*pgDeliveryAttemptRepo)(nil)

func NewDeliveryAttemptRepository(db *bun.DB) DeliveryAttemptRepository {
	return &pgDeliveryAttemptRepo{db: db}
}

func (r *pgDeliveryAttemptRepo) Upsert(ctx context.Context, a *DeliveryAttempt) error {
	row := *a
	row.ID = uuid.New()
	_, err := r.db.NewInsert().
		Model(&row).
		On("CONFLICT (notification_id) DO UPDATE").
		Set("attempt_number = EXCLUDED.attempt_number, retry_after = EXCLUDED.retry_after, updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("pg upsert delivery attempt: %w", err)
	}
	return nil
}

func (r *pgDeliveryAttemptRepo) DeleteByID(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().
		Model((*DeliveryAttempt)(nil)).
		Where("notification_id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("pg delete delivery attempt: %w", err)
	}
	return nil
}

func (r *pgDeliveryAttemptRepo) FindDueBefore(ctx context.Context, before time.Time, limit int) ([]*DeliveryAttempt, error) {
	if limit < 1 {
		limit = 1
	}
	var attempts []*DeliveryAttempt
	err := r.db.NewSelect().
		Model(&attempts).
		Where("retry_after IS NOT NULL").
		Where("retry_after <= ?", before).
		OrderExpr("retry_after ASC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("pg find due delivery attempts: %w", err)
	}
	return attempts, nil
}
