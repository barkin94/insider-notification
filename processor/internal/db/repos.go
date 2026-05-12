package db

import "context"

// DeliveryAttemptRepository is the port for delivery attempt persistence.
type DeliveryAttemptRepository interface {
	Create(ctx context.Context, a *DeliveryAttempt) error
}
