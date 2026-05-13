package db

import "context"

// DeliveryAttemptRepository is the port for delivery attempt persistence.
type DeliveryAttemptRepository interface {
	Create(ctx context.Context, a *DeliveryAttempt) error
	// FindDueRetries returns the latest failed attempt per notification whose
	// retry_after has passed. Results are ready to be re-dispatched.
	FindDueRetries(ctx context.Context) ([]*DeliveryAttempt, error)
}
