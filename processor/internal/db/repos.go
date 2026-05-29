package db

import (
	"context"

	"github.com/google/uuid"
)

// DeliveryAttemptRepository is the port for delivery attempt persistence.
type DeliveryAttemptRepository interface {
	Create(ctx context.Context, a *DeliveryAttempt) error
	// FindDueRetries returns the latest failed attempt per notification whose
	// retry_after has passed. Results are ready to be re-dispatched.
	FindDueRetries(ctx context.Context) ([]*DeliveryAttempt, error)
	// CountByNotificationID returns the number of persisted attempts for a notification.
	CountByNotificationID(ctx context.Context, id uuid.UUID) (int, error)
}

// NotificationReader is the read-only port for the notifications table.
type NotificationReader interface {
	FindScheduledDue(ctx context.Context) ([]NotificationRow, error)
	FindByIDs(ctx context.Context, ids []uuid.UUID) ([]NotificationRow, error)
}
