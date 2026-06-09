package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/barkin/insider-notification/api/internal/repository"
)

type bunNotificationRepo struct{ db *bun.DB }

var _ repository.NotificationRepository = (*bunNotificationRepo)(nil)

func NewNotificationRepository(db *bun.DB) repository.NotificationRepository {
	return &bunNotificationRepo{db: db}
}

func (r *bunNotificationRepo) Create(ctx context.Context, n *repository.Notification) error {
	_, err := r.db.NewInsert().Model(n).Exec(ctx)
	return err
}

func (r *bunNotificationRepo) GetByID(ctx context.Context, id uuid.UUID) (*repository.Notification, error) {
	n := &repository.Notification{}
	n.ID = id

	err := r.db.NewSelect().Model(n).WherePK().Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	return n, err
}

func (r *bunNotificationRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (*repository.Notification, error) {
	n := &repository.Notification{}
	n.ID = id
	n.Status = status

	err := r.db.NewUpdate().
		Model(n).
		OmitZero().
		WherePK().
		Returning("*").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (r *bunNotificationRepo) FindScheduledDue(ctx context.Context) ([]*repository.Notification, error) {
	var rows []*repository.Notification
	err := r.db.NewSelect().
		Model(&rows).
		Where("deliver_after IS NOT NULL").
		Where("deliver_after <= NOW()").
		Where("status = 'pending'").
		OrderExpr("deliver_after ASC").
		Limit(500).
		Scan(ctx)
	return rows, err
}

func applyFilters(q *bun.SelectQuery, f repository.ListFilter) *bun.SelectQuery {
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

func (r *bunNotificationRepo) List(ctx context.Context, f repository.ListFilter) ([]*repository.Notification, int, *uuid.UUID, error) {
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	total, err := applyFilters(r.db.NewSelect().Model((*repository.Notification)(nil)), f).Count(ctx)
	if err != nil {
		return nil, 0, nil, err
	}

	var ns []*repository.Notification
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
