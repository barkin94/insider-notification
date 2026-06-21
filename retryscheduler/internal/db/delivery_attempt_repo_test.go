package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	schedulerdb "github.com/barkin/insider-notification/retryscheduler/internal/db"
)

func mustNotifID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func newPayload(t *testing.T, notifID string) *schedulerdb.DeliveryAttempt {
	t.Helper()
	return &schedulerdb.DeliveryAttempt{
		NotificationID: notifID,
		Channel:        "email",
		Recipient:      "test@example.com",
		Content:        "hello",
		Priority:       "normal",
		MaxAttempts:    4,
	}
}

func TestSavePayload_isIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewDeliveryAttemptRepository(testDB)
	notifID := mustNotifID(t)

	payload := newPayload(t, notifID)
	if err := repo.Upsert(ctx, payload); err != nil {
		t.Fatalf("first SavePayload: %v", err)
	}
	if err := repo.Upsert(ctx, payload); err != nil {
		t.Fatalf("second SavePayload (idempotent): %v", err)
	}

	t.Cleanup(func() { repo.DeleteByID(ctx, notifID) }) //nolint:errcheck,gosec
}

func TestSavePayload_upsertUpdatesRetryAfter(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewDeliveryAttemptRepository(testDB)
	notifID := mustNotifID(t)

	if err := repo.Upsert(ctx, newPayload(t, notifID)); err != nil {
		t.Fatalf("first SavePayload: %v", err)
	}

	retryAt := time.Now().Add(30 * time.Second).UTC().Truncate(time.Second)
	p := newPayload(t, notifID)
	p.RetryAfter = &retryAt
	p.AttemptNumber = 2
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("second SavePayload with retry_after: %v", err)
	}

	due, err := repo.DeleteByRetryAfterBeforeReturning(ctx, retryAt.Add(time.Millisecond), 10)
	if err != nil {
		t.Fatalf("GetDue: %v", err)
	}
	var found *schedulerdb.DeliveryAttempt
	for _, a := range due {
		if a.NotificationID == notifID {
			found = a
			break
		}
	}
	if found == nil {
		t.Fatal("expected row to be due after SavePayload upsert, got none")
	}
	if found.AttemptNumber != 2 {
		t.Errorf("attempt_number = %d, want 2", found.AttemptNumber)
	}
}

func TestDelete_removesRow(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewDeliveryAttemptRepository(testDB)
	notifID := mustNotifID(t)

	retryAt := time.Now().Add(-time.Second).UTC()
	p := newPayload(t, notifID)
	p.RetryAfter = &retryAt
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("SavePayload: %v", err)
	}
	if err := repo.DeleteByID(ctx, notifID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	due, err := repo.DeleteByRetryAfterBeforeReturning(ctx, time.Now().Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("GetDue after Delete: %v", err)
	}
	for _, a := range due {
		if a.NotificationID == notifID {
			t.Error("row still appears in GetDue after Delete")
		}
	}
}

func TestDeleteByRetryAfterBeforeReturning_claimsAndRemovesRows(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewDeliveryAttemptRepository(testDB)

	ids := [2]string{mustNotifID(t), mustNotifID(t)}
	retryAt := time.Now().Add(-time.Second).UTC()
	for _, id := range ids {
		p := newPayload(t, id)
		p.RetryAfter = &retryAt
		if err := repo.Upsert(ctx, p); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	got, err := repo.DeleteByRetryAfterBeforeReturning(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("DeleteByRetryAfterBeforeReturning: %v", err)
	}
	claimed := make(map[string]bool, len(got))
	for _, a := range got {
		claimed[a.NotificationID] = true
	}
	for _, id := range ids {
		if !claimed[id] {
			t.Errorf("expected id %s to be claimed, but it was not returned", id)
		}
	}

	// Second call must not return the same rows — they were deleted.
	got2, err := repo.DeleteByRetryAfterBeforeReturning(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("second DeleteByRetryAfterBeforeReturning: %v", err)
	}
	for _, a := range got2 {
		if claimed[a.NotificationID] {
			t.Errorf("id %s returned again — row was not deleted", a.NotificationID)
		}
	}
}

func TestGetDue_respectsLimit(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewDeliveryAttemptRepository(testDB)

	ids := make([]string, 3)
	retryAt := time.Now().Add(-time.Second).UTC()
	for i := range ids {
		ids[i] = mustNotifID(t)
		p := newPayload(t, ids[i])
		p.RetryAfter = &retryAt
		if err := repo.Upsert(ctx, p); err != nil {
			t.Fatalf("SavePayload[%d]: %v", i, err)
		}
	}

	due, err := repo.DeleteByRetryAfterBeforeReturning(ctx, time.Now(), 2)
	if err != nil {
		t.Fatalf("GetDue: %v", err)
	}
	if len(due) > 2 {
		t.Errorf("expected at most 2 due entries, got %d", len(due))
	}
}
