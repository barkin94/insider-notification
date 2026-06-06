package db_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
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
	return &processordb.DeliveryAttempt{
		NotificationID: mustUUID(t).String(),
		AttemptNumber:  1,
	}
}

func TestCreate_insertsRow(t *testing.T) {
	requireRedis(t)
	ctx := context.Background()
	client := newRedisClient()
	defer client.Close() //nolint:errcheck
	repo := processordb.NewDeliveryAttemptRepository(client)

	a := newAttempt(t)
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := client.HGetAll(ctx, deliveryAttemptKey(a.NotificationID)).Result()
	if err != nil {
		t.Fatalf("hgetall: %v", err)
	}

	if got["attempt_number"] != strconv.Itoa(a.AttemptNumber) {
		t.Errorf("attempt_number: got %s, want %d", got["attempt_number"], a.AttemptNumber)
	}
}

// Second Create for the same notification_id upserts the stored attempt fields.
func TestCreate_upserts(t *testing.T) {
	requireRedis(t)
	ctx := context.Background()
	client := newRedisClient()
	defer client.Close() //nolint:errcheck
	repo := processordb.NewDeliveryAttemptRepository(client)

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

	got, err := client.HGetAll(ctx, deliveryAttemptKey(a.NotificationID)).Result()
	if err != nil {
		t.Fatalf("hgetall: %v", err)
	}
	if got["attempt_number"] != "2" {
		t.Errorf("attempt_number after upsert: got %s, want 2", got["attempt_number"])
	}
	if got["retry_after"] == "" {
		t.Error("expected retry_after to be set after upsert")
	}
}

func TestGetAttemptNumber_returnsStoredAttemptNumber(t *testing.T) {
	requireRedis(t)
	ctx := context.Background()
	client := newRedisClient()
	defer client.Close() //nolint:errcheck
	repo := processordb.NewDeliveryAttemptRepository(client)

	notifID := mustUUID(t).String()

	// No row yet → 0.
	n, err := repo.GetAttemptNumber(ctx, notifID)
	if err != nil {
		t.Fatalf("GetAttemptNumber (empty): %v", err)
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

	n, err = repo.GetAttemptNumber(ctx, notifID)
	if err != nil {
		t.Fatalf("GetAttemptNumber: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
}

func deliveryAttemptKey(id string) string {
	return "processor:delivery_attempt:{" + id + "}"
}
