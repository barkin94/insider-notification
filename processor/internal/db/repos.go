package db

import (
	"context"

	"github.com/google/uuid"
)

// NotificationReader is the read-only port for the notifications table.
type NotificationReader interface {
	FindScheduledDue(ctx context.Context) ([]NotificationRow, error)
	FindByIDs(ctx context.Context, ids []uuid.UUID) ([]NotificationRow, error)
}
