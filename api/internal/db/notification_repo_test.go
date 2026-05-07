package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/internal/shared/model"
	"github.com/google/uuid"
)

func newNotification() *model.Notification {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &model.Notification{
		ID:          mustV7(),
		Recipient:   "+905551234567",
		Channel:     model.ChannelSMS,
		Content:     "test content",
		Priority:    model.PriorityNormal,
		Status:      model.StatusPending,
		Attempts:    0,
		MaxAttempts: 4,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func mustV7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id
}

func TestNotificationRepo_Create_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := db.NewNotificationRepository(testPool)

	n := newNotification()
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != n.ID {
		t.Errorf("ID = %v, want %v", got.ID, n.ID)
	}
	if got.Recipient != n.Recipient {
		t.Errorf("Recipient = %q, want %q", got.Recipient, n.Recipient)
	}
	if got.Status != model.StatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}
}

func TestNotificationRepo_GetByID_notFound(t *testing.T) {
	ctx := context.Background()
	repo := db.NewNotificationRepository(testPool)

	_, err := repo.GetByID(ctx, mustV7())
	if err != db.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNotificationRepo_Transition(t *testing.T) {
	ctx := context.Background()
	repo := db.NewNotificationRepository(testPool)

	n := newNotification()
	repo.Create(ctx, n)

	updated, err := repo.Transition(ctx, n.ID, model.StatusPending, model.StatusProcessing)
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if updated.Status != model.StatusProcessing {
		t.Errorf("Status = %q, want processing", updated.Status)
	}
}

func TestNotificationRepo_Transition_wrongFrom(t *testing.T) {
	ctx := context.Background()
	repo := db.NewNotificationRepository(testPool)

	n := newNotification()
	repo.Create(ctx, n)

	_, err := repo.Transition(ctx, n.ID, model.StatusDelivered, model.StatusProcessing)
	if err != db.ErrTransitionFailed {
		t.Errorf("expected ErrTransitionFailed, got %v", err)
	}
}

func TestNotificationRepo_IncrementAttempts(t *testing.T) {
	ctx := context.Background()
	repo := db.NewNotificationRepository(testPool)

	n := newNotification()
	repo.Create(ctx, n)

	if err := repo.IncrementAttempts(ctx, n.ID); err != nil {
		t.Fatalf("IncrementAttempts: %v", err)
	}

	got, _ := repo.GetByID(ctx, n.ID)
	if got.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", got.Attempts)
	}
}

func TestNotificationRepo_List_pagination(t *testing.T) {
	ctx := context.Background()
	repo := db.NewNotificationRepository(testPool)

	batchID := mustV7()
	for i := 0; i < 5; i++ {
		n := newNotification()
		n.BatchID = &batchID
		repo.Create(ctx, n)
	}

	results, total, err := repo.List(ctx, db.ListFilter{BatchID: &batchID, Page: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
}

func TestNotificationRepo_List_filterByStatus(t *testing.T) {
	ctx := context.Background()
	repo := db.NewNotificationRepository(testPool)

	batchID := mustV7()
	for i := 0; i < 3; i++ {
		n := newNotification()
		n.BatchID = &batchID
		repo.Create(ctx, n)
	}
	n := newNotification()
	n.BatchID = &batchID
	n.Status = model.StatusDelivered
	repo.Create(ctx, n)

	results, total, err := repo.List(ctx, db.ListFilter{
		BatchID: &batchID,
		Status:  model.StatusDelivered,
		Page:    1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Errorf("expected 1 delivered, got total=%d len=%d", total, len(results))
	}
}
