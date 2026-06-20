package db

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// DeliveryAttemptRepository is the port for delivery attempt persistence and retry scheduling.
type DeliveryAttemptRepository interface {
	// Upsert inserts a delivery attempt row; on conflict updates attempt_number,
	// retry_after, and updated_at so the retry dispatcher picks up the latest state.
	Upsert(ctx context.Context, a *DeliveryAttempt) error
	// UpsertAll is the batch variant of Upsert — all rows are written in a single round trip.
	UpsertAll(ctx context.Context, attempts []*DeliveryAttempt) error
	DeleteByID(ctx context.Context, id string) error
	// FindAndDeleteDueBefore atomically claims and removes up to limit attempts whose
	// retry_after is at or before before, using SELECT ... FOR UPDATE SKIP LOCKED so
	// concurrent callers never receive the same rows.
	FindAndDeleteDueBefore(ctx context.Context, before time.Time, limit int) ([]*DeliveryAttempt, error)
}

type pgDeliveryAttemptRepo struct{ db *bun.DB }

var _ DeliveryAttemptRepository = (*pgDeliveryAttemptRepo)(nil)

func NewDeliveryAttemptRepository(db *bun.DB) DeliveryAttemptRepository {
	return &pgDeliveryAttemptRepo{db: db}
}

func (r *pgDeliveryAttemptRepo) UpsertAll(ctx context.Context, attempts []*DeliveryAttempt) error {
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

func (r *pgDeliveryAttemptRepo) Upsert(ctx context.Context, a *DeliveryAttempt) error {
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
		Model((*DeliveryAttempt)(nil)).
		Where("notification_id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("pg delete delivery attempt: %w", err)
	}
	return nil
}

func (r *pgDeliveryAttemptRepo) FindAndDeleteDueBefore(ctx context.Context, before time.Time, limit int) ([]*DeliveryAttempt, error) {
	if limit < 1 {
		limit = 1
	}
	var attempts []*DeliveryAttempt
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
