package db

import (
	"context"
	"time"

	"github.com/barkin/insider-notification/internal/shared/model"
	"github.com/google/uuid"
)

// ListFilter holds query parameters for listing notifications.
type ListFilter struct {
	Status   string
	Channel  string
	BatchID  *uuid.UUID
	DateFrom *time.Time
	DateTo   *time.Time
	Page     int
	PageSize int
	Sort     string
	Order    string
}

// NotificationRepository is the port for notification persistence.
type NotificationRepository interface {
	Create(ctx context.Context, n *model.Notification) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error)
	List(ctx context.Context, f ListFilter) ([]*model.Notification, int, error)
	Transition(ctx context.Context, id uuid.UUID, from, to string) (*model.Notification, error)
	IncrementAttempts(ctx context.Context, id uuid.UUID) error
}

// DeliveryAttemptRepository is the port for delivery attempt persistence.
type DeliveryAttemptRepository interface {
	Create(ctx context.Context, a *model.DeliveryAttempt) error
	ListByNotificationID(ctx context.Context, notificationID uuid.UUID) ([]*model.DeliveryAttempt, error)
}

