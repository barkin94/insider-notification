package db

import (
	"context"

	"github.com/barkin/insider-notification/internal/shared/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxDeliveryAttemptRepo struct{ pool *pgxpool.Pool }

func NewDeliveryAttemptRepository(pool *pgxpool.Pool) DeliveryAttemptRepository {
	return &pgxDeliveryAttemptRepo{pool: pool}
}

func (r *pgxDeliveryAttemptRepo) Create(ctx context.Context, a *model.DeliveryAttempt) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO delivery_attempts
			(id, notification_id, attempt_number, status, http_status_code,
			 provider_response, error_message, latency_ms, attempted_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (notification_id, attempt_number) DO NOTHING`,
		a.ID, a.NotificationID, a.AttemptNumber, a.Status, a.HTTPStatusCode,
		a.ProviderResponse, a.ErrorMessage, a.LatencyMS, a.AttemptedAt,
	)
	return err
}

func (r *pgxDeliveryAttemptRepo) ListByNotificationID(ctx context.Context, notificationID uuid.UUID) ([]*model.DeliveryAttempt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT * FROM delivery_attempts
		WHERE notification_id = $1
		ORDER BY attempt_number ASC`, notificationID)
	if err != nil {
		return nil, err
	}
	attempts, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.DeliveryAttempt])
	if err != nil {
		return nil, err
	}
	result := make([]*model.DeliveryAttempt, len(attempts))
	for i := range attempts {
		result[i] = &attempts[i]
	}
	return result, nil
}
