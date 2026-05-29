package db_test

import (
	"context"
	"testing"
	"time"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/google/uuid"
)

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newAttempt(t *testing.T) *processordb.DeliveryAttempt {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	a := &processordb.DeliveryAttempt{
		NotificationID: mustUUID(t),
		AttemptNumber:  1,
	}
	a.ID = mustUUID(t)
	a.CreatedAt = now
	a.UpdatedAt = now
	return a
}

func TestCreate_insertsRow(t *testing.T) {
	ctx := context.Background()
	repo := processordb.NewDeliveryAttemptRepository(testDB)

	a := newAttempt(t)
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var got processordb.DeliveryAttempt
	err := testDB.NewSelect().Model(&got).Where("id = ?", a.ID).Scan(ctx)
	if err != nil {
		t.Fatalf("select: %v", err)
	}

	if got.NotificationID != a.NotificationID {
		t.Errorf("notification_id: got %v, want %v", got.NotificationID, a.NotificationID)
	}
	if got.AttemptNumber != a.AttemptNumber {
		t.Errorf("attempt_number: got %d, want %d", got.AttemptNumber, a.AttemptNumber)
	}
}

// Second Create for the same notification_id upserts: updates attempt_number, status, retry_after.
func TestCreate_upserts(t *testing.T) {
	ctx := context.Background()
	repo := processordb.NewDeliveryAttemptRepository(testDB)

	a := newAttempt(t)
	a.AttemptNumber = 1
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	retryAfter := time.Now().Add(time.Minute).UTC().Truncate(time.Millisecond)
	a2 := newAttempt(t)
	a2.NotificationID = a.NotificationID // same notification
	a2.AttemptNumber = 2
	a2.RetryAfter = &retryAfter
	if err := repo.Create(ctx, a2); err != nil {
		t.Fatalf("second Create (upsert): %v", err)
	}

	var count int
	err := testDB.NewSelect().
		TableExpr("delivery_attempts").
		ColumnExpr("count(*)").
		Where("notification_id = ?", a.NotificationID).
		Scan(ctx, &count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after upsert, got %d", count)
	}

	var got processordb.DeliveryAttempt
	if err := testDB.NewSelect().Model(&got).Where("notification_id = ?", a.NotificationID).Scan(ctx); err != nil {
		t.Fatalf("select after upsert: %v", err)
	}
	if got.AttemptNumber != 2 {
		t.Errorf("attempt_number after upsert: got %d, want 2", got.AttemptNumber)
	}
	if got.RetryAfter == nil {
		t.Error("expected retry_after to be set after upsert")
	}
}

func TestCountByNotificationID_returnsAttemptNumber(t *testing.T) {
	ctx := context.Background()
	repo := processordb.NewDeliveryAttemptRepository(testDB)

	notifID := mustUUID(t)

	// No row yet → 0.
	n, err := repo.CountByNotificationID(ctx, notifID)
	if err != nil {
		t.Fatalf("CountByNotificationID (empty): %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 before any attempt, got %d", n)
	}

	a := newAttempt(t)
	a.NotificationID = notifID
	a.AttemptNumber = 3
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	n, err = repo.CountByNotificationID(ctx, notifID)
	if err != nil {
		t.Fatalf("CountByNotificationID: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
}
