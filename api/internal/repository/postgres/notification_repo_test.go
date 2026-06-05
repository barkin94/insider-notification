package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/barkin/insider-notification/api/internal/repository"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/google/uuid"
)

func newNotification() *repository.Notification {
	now := time.Now().UTC().Truncate(time.Millisecond)
	n := &repository.Notification{
		Recipient:   "+905551234567",
		Channel:     string(model.ChannelSMS),
		Content:     "test content",
		Priority:    string(model.PriorityNormal),
		Status:      string(model.StatusPending),
		Attempts:    0,
		MaxAttempts: 4,
	}
	n.ID = mustV7()
	n.CreatedAt = now
	n.UpdatedAt = now
	return n
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
	repo := NewNotificationRepository(testDB)

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
	if got.Status != string(model.StatusPending) {
		t.Errorf("Status = %q, want pending", got.Status)
	}
}

func TestNotificationRepo_GetByID_notFound(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	_, err := repo.GetByID(ctx, mustV7())
	if err != repository.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNotificationRepo_Transition(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	n := newNotification()
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	updated, err := repo.Transition(ctx, n.ID, string(model.StatusPending), string(model.StatusCancelled))
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if updated.Status != string(model.StatusCancelled) {
		t.Errorf("Status = %q, want cancelled", updated.Status)
	}
}

func TestNotificationRepo_Transition_wrongFrom(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	n := newNotification()
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	_, err := repo.Transition(ctx, n.ID, string(model.StatusDelivered), string(model.StatusCancelled))
	if err != repository.ErrTransitionFailed {
		t.Errorf("expected ErrTransitionFailed, got %v", err)
	}
}

func TestNotificationRepo_IncrementAttempts(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	n := newNotification()
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	if err := repo.IncrementAttempts(ctx, n.ID); err != nil {
		t.Fatalf("IncrementAttempts: %v", err)
	}

	got, _ := repo.GetByID(ctx, n.ID)
	if got.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", got.Attempts)
	}
}

func TestList_offset_pagination(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	batchID := mustV7()
	for i := 0; i < 5; i++ {
		n := newNotification()
		n.BatchID = &batchID
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("create notification: %v", err)
		}
	}

	results, total, nextCursor, err := repo.List(ctx, repository.ListFilter{BatchID: &batchID, Page: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
	if nextCursor != nil {
		t.Error("offset mode should not return nextCursor")
	}
}

func TestList_offset_filterByStatus(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	batchID := mustV7()
	for i := 0; i < 3; i++ {
		n := newNotification()
		n.BatchID = &batchID
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("create notification: %v", err)
		}
	}
	n := newNotification()
	n.BatchID = &batchID
	n.Status = string(model.StatusDelivered)
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	results, total, _, err := repo.List(ctx, repository.ListFilter{
		BatchID: &batchID,
		Status:  string(model.StatusDelivered),
		Page:    1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Errorf("expected 1 delivered, got total=%d len=%d", total, len(results))
	}
}

func seed5(t *testing.T, repo repository.NotificationRepository, batchID uuid.UUID) []*repository.Notification {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		n := newNotification()
		n.BatchID = &batchID
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	maxUUID := uuid.UUID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	all, _, _, err := repo.List(ctx, repository.ListFilter{BatchID: &batchID, PageSize: 10, CursorID: &maxUUID})
	if err != nil {
		t.Fatalf("seed fetch: %v", err)
	}
	return all
}

func TestList_cursor_firstPage(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	batchID := mustV7()
	all := seed5(t, repo, batchID)

	maxUUID := uuid.UUID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	results, total, nextCursor, err := repo.List(ctx, repository.ListFilter{
		BatchID:  &batchID,
		PageSize: 3,
		CursorID: &maxUUID,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
	if nextCursor == nil {
		t.Error("nextCursor should not be nil when more pages exist")
	}
	seededIDs := map[uuid.UUID]bool{}
	for _, n := range all {
		seededIDs[n.ID] = true
	}
	for _, n := range results {
		if !seededIDs[n.ID] {
			t.Errorf("result ID %v not in seeded set", n.ID)
		}
	}
}

func TestList_cursor_secondPage(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	batchID := mustV7()
	all := seed5(t, repo, batchID)

	cursorID := all[2].ID
	page2, total, nextCursor, err := repo.List(ctx, repository.ListFilter{
		BatchID:  &batchID,
		PageSize: 3,
		CursorID: &cursorID,
	})
	if err != nil {
		t.Fatalf("page2 List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(page2) != 2 {
		t.Errorf("len(page2) = %d, want 2", len(page2))
	}
	if nextCursor != nil {
		t.Error("nextCursor should be nil on last page")
	}
	if page2[0].ID != all[3].ID || page2[1].ID != all[4].ID {
		t.Error("page2 items do not match expected IDs")
	}
}

func TestList_cursor_lastPage_noNextCursor(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	batchID := mustV7()
	seed5(t, repo, batchID)

	maxUUID := uuid.UUID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	results, total, nextCursor, err := repo.List(ctx, repository.ListFilter{
		BatchID:  &batchID,
		PageSize: 10,
		CursorID: &maxUUID,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(results) != 5 {
		t.Errorf("len(results) = %d, want 5", len(results))
	}
	if nextCursor != nil {
		t.Error("nextCursor should be nil when all results fit in one page")
	}
}

func TestList_cursor_filtersPreserved(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepository(testDB)

	batchID := mustV7()
	for i := 0; i < 3; i++ {
		n := newNotification()
		n.BatchID = &batchID
		n.Channel = string(model.ChannelSMS)
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("create notification: %v", err)
		}
	}
	for i := 0; i < 4; i++ {
		n := newNotification()
		n.BatchID = &batchID
		n.Channel = string(model.ChannelEmail)
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("create notification: %v", err)
		}
	}

	maxUUID := uuid.UUID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	results, total, _, err := repo.List(ctx, repository.ListFilter{
		BatchID:  &batchID,
		Channel:  string(model.ChannelSMS),
		PageSize: 10,
		CursorID: &maxUUID,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
	for _, n := range results {
		if n.Channel != string(model.ChannelSMS) {
			t.Errorf("expected channel sms, got %q", n.Channel)
		}
	}
}
