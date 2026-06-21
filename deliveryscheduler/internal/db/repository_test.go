package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	schedulerdb "github.com/barkin/insider-notification/deliveryscheduler/internal/db"
)

func mustNotifID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func newScheduledNotification(t *testing.T, notifID string, scheduledAt *time.Time) *schedulerdb.ScheduledNotification {
	t.Helper()
	return &schedulerdb.ScheduledNotification{
		NotificationID: notifID,
		ScheduledAt:    scheduledAt,
	}
}

func TestUpsertAll_insertsMultipleRows(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewScheduledNotificationRepository(testDB)

	ids := [3]string{mustNotifID(t), mustNotifID(t), mustNotifID(t)}
	scheduledAt := time.Now().Add(-time.Hour).UTC()
	notifications := make([]*schedulerdb.ScheduledNotification, len(ids))
	for i, id := range ids {
		notifications[i] = newScheduledNotification(t, id, &scheduledAt)
	}

	if err := repo.UpsertAll(ctx, notifications); err != nil {
		t.Fatalf("UpsertAll: %v", err)
	}

	// Verify all were inserted by trying to delete them
	got, err := repo.DeleteByScheduledAtBeforeReturning(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("DeleteByScheduledAtBeforeReturning: %v", err)
	}
	if len(got) != len(ids) {
		t.Errorf("expected %d rows, got %d", len(ids), len(got))
	}
}

func TestUpsertAll_isIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewScheduledNotificationRepository(testDB)

	notifID := mustNotifID(t)
	scheduledAt := time.Now().Add(-time.Hour).UTC()
	notification := newScheduledNotification(t, notifID, &scheduledAt)

	if err := repo.UpsertAll(ctx, []*schedulerdb.ScheduledNotification{notification}); err != nil {
		t.Fatalf("first UpsertAll: %v", err)
	}

	// Upsert again with same ID but different time
	newTime := time.Now().Add(-time.Minute).UTC()
	notification.ScheduledAt = &newTime
	if err := repo.UpsertAll(ctx, []*schedulerdb.ScheduledNotification{notification}); err != nil {
		t.Fatalf("second UpsertAll: %v", err)
	}

	// Verify the time was updated
	got, err := repo.DeleteByScheduledAtBeforeReturning(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("DeleteByScheduledAtBeforeReturning: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].ScheduledAt == nil || *got[0].ScheduledAt != newTime {
		t.Errorf("scheduled_at not updated: got %v, want %v", got[0].ScheduledAt, newTime)
	}
}

func TestDeleteByScheduledAtBeforeReturning_claimsAndRemoves(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewScheduledNotificationRepository(testDB)

	ids := [3]string{mustNotifID(t), mustNotifID(t), mustNotifID(t)}
	pastTime := time.Now().Add(-time.Hour).UTC()
	futureTime := time.Now().Add(time.Hour).UTC()

	notifications := []*schedulerdb.ScheduledNotification{
		newScheduledNotification(t, ids[0], &pastTime),
		newScheduledNotification(t, ids[1], &pastTime),
		newScheduledNotification(t, ids[2], &futureTime),
	}

	if err := repo.UpsertAll(ctx, notifications); err != nil {
		t.Fatalf("UpsertAll: %v", err)
	}

	// Get only past notifications
	got, err := repo.DeleteByScheduledAtBeforeReturning(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("DeleteByScheduledAtBeforeReturning: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("expected 2 rows, got %d", len(got))
	}

	// Verify they were deleted
	got2, err := repo.DeleteByScheduledAtBeforeReturning(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("second DeleteByScheduledAtBeforeReturning: %v", err)
	}
	if len(got2) > 0 {
		t.Errorf("expected 0 rows after delete, got %d", len(got2))
	}
}

func TestDeleteByScheduledAtBeforeReturning_respectsLimit(t *testing.T) {
	ctx := context.Background()
	repo := schedulerdb.NewScheduledNotificationRepository(testDB)

	ids := make([]string, 5)
	pastTime := time.Now().Add(-time.Hour).UTC()
	notifications := make([]*schedulerdb.ScheduledNotification, len(ids))

	for i := range ids {
		ids[i] = mustNotifID(t)
		notifications[i] = newScheduledNotification(t, ids[i], &pastTime)
	}

	if err := repo.UpsertAll(ctx, notifications); err != nil {
		t.Fatalf("UpsertAll: %v", err)
	}

	// Delete with limit of 2
	got, err := repo.DeleteByScheduledAtBeforeReturning(ctx, time.Now(), 2)
	if err != nil {
		t.Fatalf("DeleteByScheduledAtBeforeReturning: %v", err)
	}

	if len(got) > 2 {
		t.Errorf("expected at most 2 rows, got %d", len(got))
	}
}
