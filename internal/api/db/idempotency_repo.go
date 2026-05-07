package db

import (
	"context"
	"errors"

	"github.com/barkin/insider-notification/internal/shared/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxIdempotencyRepo struct{ pool *pgxpool.Pool }

func NewIdempotencyRepository(pool *pgxpool.Pool) IdempotencyRepository {
	return &pgxIdempotencyRepo{pool: pool}
}

func (r *pgxIdempotencyRepo) GetByKey(ctx context.Context, key string) (*model.IdempotencyKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT key, notification_id, key_type, expires_at, created_at
		FROM idempotency_keys
		WHERE key = $1 AND expires_at > NOW()`, key)
	if err != nil {
		return nil, err
	}
	k, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.IdempotencyKey])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *pgxIdempotencyRepo) Create(ctx context.Context, k *model.IdempotencyKey) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO idempotency_keys (key, notification_id, key_type, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT DO NOTHING`,
		k.Key, k.NotificationID, k.KeyType, k.ExpiresAt, k.CreatedAt,
	)
	return err
}

func (r *pgxIdempotencyRepo) DeleteExpired(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE expires_at < NOW()`)
	return err
}
