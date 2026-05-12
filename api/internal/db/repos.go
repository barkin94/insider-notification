package db

import (
	"context"
	"time"

	apimodel "github.com/barkin/insider-notification/api/internal/model"
	"github.com/google/uuid"
)

// ListFilter holds query parameters for listing notifications.
// Set CursorID for keyset pagination; leave nil for offset pagination.
type ListFilter struct {
	Status   string
	Channel  string
	BatchID  *uuid.UUID
	DateFrom *time.Time
	DateTo   *time.Time
	PageSize int
	// keyset pagination — takes precedence over Page/Sort/Order when set
	CursorID *uuid.UUID
	// offset pagination
	Page  int
	Sort  string
	Order string
}

// NotificationRepository is the port for notification persistence.
type NotificationRepository interface {
	Create(ctx context.Context, n *apimodel.Notification) error
	GetByID(ctx context.Context, id uuid.UUID) (*apimodel.Notification, error)
	List(ctx context.Context, f ListFilter) ([]*apimodel.Notification, int, *uuid.UUID, error)
	Transition(ctx context.Context, id uuid.UUID, from, to string) (*apimodel.Notification, error)
	IncrementAttempts(ctx context.Context, id uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}
