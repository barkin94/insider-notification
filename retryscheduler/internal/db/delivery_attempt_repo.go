package db

import (
	"context"
	"time"
)

// DeliveryAttemptRepository is the port for delivery attempt persistence and retry scheduling.
type DeliveryAttemptRepository interface {
	// Upsert inserts a delivery attempt row; on conflict updates attempt_number,
	// retry_after, and updated_at so the retry dispatcher picks up the latest state.
	Upsert(ctx context.Context, a *DeliveryAttempt) error
	// UpsertAll is the batch variant of Upsert — all rows are written in a single round trip.
	UpsertAll(ctx context.Context, attempts []*DeliveryAttempt) error
	DeleteByID(ctx context.Context, id string) error
	// DeleteByRetryAfterBeforeReturning atomically claims and removes up to limit attempts whose
	// retry_after is at or before before, using SELECT ... FOR UPDATE SKIP LOCKED so
	// concurrent callers never receive the same rows.
	DeleteByRetryAfterBeforeReturning(ctx context.Context, before time.Time, limit int) ([]*DeliveryAttempt, error)
}
