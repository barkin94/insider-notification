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
		Status:         "delivered",
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
	if got.Status != a.Status {
		t.Errorf("status: got %s, want %s", got.Status, a.Status)
	}
}

func TestCreate_idempotent(t *testing.T) {
	ctx := context.Background()
	repo := processordb.NewDeliveryAttemptRepository(testDB)

	a := newAttempt(t)
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("second Create (duplicate): %v", err)
	}

	var count int
	err := testDB.NewSelect().
		TableExpr("processor.delivery_attempts").
		ColumnExpr("count(*)").
		Where("notification_id = ? AND attempt_number = ?", a.NotificationID, a.AttemptNumber).
		Scan(ctx, &count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}
