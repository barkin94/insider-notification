package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/barkin/insider-notification/internal/api/db"
	"github.com/barkin/insider-notification/internal/shared/model"
)

func TestIdempotencyRepo_GetByKey_miss(t *testing.T) {
	ctx := context.Background()
	repo := db.NewIdempotencyRepository(testPool)

	_, err := repo.GetByKey(ctx, "nonexistent-key")
	if err != db.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIdempotencyRepo_Create_GetByKey(t *testing.T) {
	ctx := context.Background()
	nRepo := db.NewNotificationRepository(testPool)
	iRepo := db.NewIdempotencyRepository(testPool)

	n := newNotification()
	nRepo.Create(ctx, n)

	k := &model.IdempotencyKey{
		Key:            "test-key-" + mustV7().String(),
		NotificationID: n.ID,
		KeyType:        "client",
		ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
		CreatedAt:      time.Now().UTC(),
	}
	if err := iRepo.Create(ctx, k); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := iRepo.GetByKey(ctx, k.Key)
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if got.NotificationID != n.ID {
		t.Errorf("NotificationID = %v, want %v", got.NotificationID, n.ID)
	}
}

func TestIdempotencyRepo_GetByKey_expired(t *testing.T) {
	ctx := context.Background()
	nRepo := db.NewNotificationRepository(testPool)
	iRepo := db.NewIdempotencyRepository(testPool)

	n := newNotification()
	nRepo.Create(ctx, n)

	k := &model.IdempotencyKey{
		Key:            "expired-key-" + mustV7().String(),
		NotificationID: n.ID,
		KeyType:        "content_hash",
		ExpiresAt:      time.Now().UTC().Add(-1 * time.Hour), // already expired
		CreatedAt:      time.Now().UTC().Add(-2 * time.Hour),
	}
	iRepo.Create(ctx, k)

	_, err := iRepo.GetByKey(ctx, k.Key)
	if err != db.ErrNotFound {
		t.Errorf("expected ErrNotFound for expired key, got %v", err)
	}
}

func TestIdempotencyRepo_DeleteExpired(t *testing.T) {
	ctx := context.Background()
	nRepo := db.NewNotificationRepository(testPool)
	iRepo := db.NewIdempotencyRepository(testPool)

	n := newNotification()
	nRepo.Create(ctx, n)

	expiredKey := "del-expired-" + mustV7().String()
	iRepo.Create(ctx, &model.IdempotencyKey{
		Key:            expiredKey,
		NotificationID: n.ID,
		KeyType:        "client",
		ExpiresAt:      time.Now().UTC().Add(-1 * time.Hour),
		CreatedAt:      time.Now().UTC(),
	})

	if err := iRepo.DeleteExpired(ctx); err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}

	_, err := iRepo.GetByKey(ctx, expiredKey)
	if err != db.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
