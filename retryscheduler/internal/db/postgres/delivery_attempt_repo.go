package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	schedulerdb "github.com/barkin94/insider-notification/retryscheduler/internal/db"
)

type pgDeliveryAttemptRepo struct{ db *bun.DB }

var _ schedulerdb.DeliveryAttemptRepository = (*pgDeliveryAttemptRepo)(nil)

func NewDeliveryAttemptRepository(db *bun.DB) schedulerdb.DeliveryAttemptRepository {
	return &pgDeliveryAttemptRepo{db: db}
}

func (r *pgDeliveryAttemptRepo) UpsertAll(ctx context.Context, attempts []*schedulerdb.DeliveryAttempt) error {
	if len(attempts) == 0 {
		return nil
	}
	_, err := r.db.NewInsert().
		Model(&attempts).
		On("CONFLICT (notification_id) DO UPDATE").
		Set("attempt_number = EXCLUDED.attempt_number, retry_after = EXCLUDED.retry_after").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("pg batch upsert delivery attempts: %w", err)
	}
	return nil
}

func (r *pgDeliveryAttemptRepo) Upsert(ctx context.Context, a *schedulerdb.DeliveryAttempt) error {
	row := *a
	_, err := r.db.NewInsert().
		Model(&row).
		On("CONFLICT (notification_id) DO UPDATE").
		Set("attempt_number = EXCLUDED.attempt_number, retry_after = EXCLUDED.retry_after").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("pg upsert delivery attempt: %w", err)
	}
	return nil
}

func (r *pgDeliveryAttemptRepo) DeleteByID(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().
		Model((*schedulerdb.DeliveryAttempt)(nil)).
		Where("notification_id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("pg delete delivery attempt: %w", err)
	}
	return nil
}

func (r *pgDeliveryAttemptRepo) DeleteByRetryAfterBeforeReturning(ctx context.Context, before time.Time, limit int) ([]*schedulerdb.DeliveryAttempt, error) {
	if limit < 1 {
		limit = 1
	}
	var attempts []*schedulerdb.DeliveryAttempt
	err := r.db.NewRaw(`
		DELETE FROM delivery_attempts
		WHERE notification_id IN (
			SELECT notification_id FROM delivery_attempts
			WHERE retry_after IS NOT NULL AND retry_after <= ?
			ORDER BY retry_after ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *
		`,
		before, limit,
	).Scan(ctx, &attempts)
	if err != nil {
		return nil, fmt.Errorf("pg find and delete due delivery attempts: %w", err)
	}
	return attempts, nil
}
