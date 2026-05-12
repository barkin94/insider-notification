package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	apimodel "github.com/barkin/insider-notification/api/internal/model"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type bunNotificationRepo struct{ db *bun.DB }

func NewNotificationRepository(db *bun.DB) NotificationRepository {
	return &bunNotificationRepo{db: db}
}

func (r *bunNotificationRepo) Create(ctx context.Context, n *apimodel.Notification) error {
	_, err := r.db.NewInsert().Model(n).Exec(ctx)
	return err
}

func (r *bunNotificationRepo) GetByID(ctx context.Context, id uuid.UUID) (*apimodel.Notification, error) {
	n := new(apimodel.Notification)
	err := r.db.NewSelect().Model(n).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return n, err
}

func (r *bunNotificationRepo) Transition(ctx context.Context, id uuid.UUID, from, to string) (*apimodel.Notification, error) {
	n := new(apimodel.Notification)
	err := r.db.NewRaw(`
		UPDATE notifications SET status = ?, updated_at = NOW()
		WHERE id = ? AND status = ?
		RETURNING *`, to, id, from).Scan(ctx, n)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTransitionFailed
	}
	return n, err
}

func (r *bunNotificationRepo) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.NewRaw(
		`UPDATE notifications SET attempts = attempts + 1, updated_at = NOW() WHERE id = ?`, id,
	).Exec(ctx)
	return err
}

func (r *bunNotificationRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.db.NewRaw(
		`UPDATE notifications SET status = ?, updated_at = NOW() WHERE id = ?`, status, id,
	).Exec(ctx)
	return err
}

func applyFilters(q *bun.SelectQuery, f ListFilter) *bun.SelectQuery {
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Channel != "" {
		q = q.Where("channel = ?", f.Channel)
	}
	if f.BatchID != nil {
		q = q.Where("batch_id = ?", f.BatchID)
	}
	if f.DateFrom != nil {
		q = q.Where("created_at >= ?", f.DateFrom)
	}
	if f.DateTo != nil {
		q = q.Where("created_at <= ?", f.DateTo)
	}
	return q
}

func (r *bunNotificationRepo) List(ctx context.Context, f ListFilter) ([]*apimodel.Notification, int, *uuid.UUID, error) {
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	total, err := applyFilters(r.db.NewSelect().Model((*apimodel.Notification)(nil)), f).Count(ctx)
	if err != nil {
		return nil, 0, nil, err
	}

	var ns []*apimodel.Notification
	q := applyFilters(r.db.NewSelect().Model(&ns), f)

	if f.CursorID != nil {
		q = q.Where("id < ?", f.CursorID).OrderExpr("id DESC").Limit(pageSize + 1)
	} else {
		sort := "created_at"
		if f.Sort == "updated_at" {
			sort = "updated_at"
		}
		order := "DESC"
		if strings.EqualFold(f.Order, "asc") {
			order = "ASC"
		}
		page := f.Page
		if page < 1 {
			page = 1
		}
		q = q.OrderExpr(sort + " " + order).Limit(pageSize).Offset((page - 1) * pageSize)
	}

	if err := q.Scan(ctx); err != nil {
		return nil, 0, nil, err
	}

	var nextCursor *uuid.UUID
	if f.CursorID != nil && len(ns) == pageSize+1 {
		id := ns[pageSize-1].ID
		nextCursor = &id
		ns = ns[:pageSize]
	}

	return ns, total, nextCursor, nil
}
