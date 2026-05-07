package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/barkin/insider-notification/internal/shared/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxNotificationRepo struct{ pool *pgxpool.Pool }

func NewNotificationRepository(pool *pgxpool.Pool) NotificationRepository {
	return &pgxNotificationRepo{pool: pool}
}

func (r *pgxNotificationRepo) Create(ctx context.Context, n *model.Notification) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notifications
			(id, batch_id, recipient, channel, content, priority, status,
			 idempotency_key, deliver_after, attempts, max_attempts, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		n.ID, n.BatchID, n.Recipient, n.Channel, n.Content, n.Priority, n.Status,
		n.IdempotencyKey, n.DeliverAfter, n.Attempts, n.MaxAttempts, n.Metadata,
		n.CreatedAt, n.UpdatedAt,
	)
	return err
}

func (r *pgxNotificationRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	rows, err := r.pool.Query(ctx, `SELECT * FROM notifications WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	n, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Notification])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *pgxNotificationRepo) Transition(ctx context.Context, id uuid.UUID, from, to string) (*model.Notification, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE notifications SET status = $1, updated_at = NOW()
		WHERE id = $2 AND status = $3
		RETURNING *`, to, id, from)
	if err != nil {
		return nil, err
	}
	n, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Notification])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTransitionFailed
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *pgxNotificationRepo) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET attempts = attempts + 1, updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *pgxNotificationRepo) List(ctx context.Context, f ListFilter) ([]*model.Notification, int, error) {
	conds := []string{}
	args := []any{}
	n := 1

	if f.Status != "" {
		conds = append(conds, fmt.Sprintf("status = $%d", n))
		args = append(args, f.Status)
		n++
	}
	if f.Channel != "" {
		conds = append(conds, fmt.Sprintf("channel = $%d", n))
		args = append(args, f.Channel)
		n++
	}
	if f.BatchID != nil {
		conds = append(conds, fmt.Sprintf("batch_id = $%d", n))
		args = append(args, *f.BatchID)
		n++
	}
	if f.DateFrom != nil {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", n))
		args = append(args, *f.DateFrom)
		n++
	}
	if f.DateTo != nil {
		conds = append(conds, fmt.Sprintf("created_at <= $%d", n))
		args = append(args, *f.DateTo)
		n++
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM notifications %s", where), args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	sort := "created_at"
	if f.Sort == "updated_at" {
		sort = "updated_at"
	}
	order := "DESC"
	if strings.EqualFold(f.Order, "asc") {
		order = "ASC"
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}

	dataArgs := append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT * FROM notifications %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`, where, sort, order, n, n+1), dataArgs...)
	if err != nil {
		return nil, 0, err
	}

	ns, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.Notification])
	if err != nil {
		return nil, 0, err
	}
	result := make([]*model.Notification, len(ns))
	for i := range ns {
		result[i] = &ns[i]
	}
	return result, total, nil
}
