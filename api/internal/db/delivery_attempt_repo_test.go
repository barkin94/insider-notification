package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/shared/model"
)

func TestDeliveryAttemptRepo_Create_idempotent(t *testing.T) {
	ctx := context.Background()
	nRepo := db.NewNotificationRepository(testPool)
	aRepo := db.NewDeliveryAttemptRepository(testPool)

	n := newNotification()
	nRepo.Create(ctx, n)

	statusCode := 202
	latency := 143
	a := &model.DeliveryAttempt{
		ID:             mustV7(),
		NotificationID: n.ID,
		AttemptNumber:  1,
		Status:         "success",
		HTTPStatusCode: &statusCode,
		LatencyMS:      &latency,
		AttemptedAt:    time.Now().UTC(),
	}

	if err := aRepo.Create(ctx, a); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// duplicate insert must not error
	if err := aRepo.Create(ctx, a); err != nil {
		t.Fatalf("second Create (duplicate): %v", err)
	}

	attempts, err := aRepo.ListByNotificationID(ctx, n.ID)
	if err != nil {
		t.Fatalf("ListByNotificationID: %v", err)
	}
	if len(attempts) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(attempts))
	}
}

func TestDeliveryAttemptRepo_ListByNotificationID(t *testing.T) {
	ctx := context.Background()
	nRepo := db.NewNotificationRepository(testPool)
	aRepo := db.NewDeliveryAttemptRepository(testPool)

	n := newNotification()
	nRepo.Create(ctx, n)

	for i := 1; i <= 3; i++ {
		code := 503
		latency := 100 * i
		aRepo.Create(ctx, &model.DeliveryAttempt{
			ID:             mustV7(),
			NotificationID: n.ID,
			AttemptNumber:  i,
			Status:         "failed",
			HTTPStatusCode: &code,
			LatencyMS:      &latency,
			AttemptedAt:    time.Now().UTC(),
		})
	}

	attempts, err := aRepo.ListByNotificationID(ctx, n.ID)
	if err != nil {
		t.Fatalf("ListByNotificationID: %v", err)
	}
	if len(attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(attempts))
	}
}
