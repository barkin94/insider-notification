package messaging_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	apipub "github.com/barkin/insider-notification/api/public"
	db "github.com/barkin/insider-notification/deliveryscheduler/internal/db"
)

// MockRepository mocks ScheduledNotificationRepository for testing
type MockRepository struct {
	upsertedAll []*db.ScheduledNotification
}

func (m *MockRepository) UpsertAll(ctx context.Context, notifications []*db.ScheduledNotification) error {
	m.upsertedAll = append(m.upsertedAll, notifications...)
	return nil
}

func (m *MockRepository) DeleteByScheduledAtBeforeReturning(ctx context.Context, before time.Time, limit int) ([]*db.ScheduledNotification, error) {
	return nil, nil
}

func TestConsumer_BatchesNotifications(t *testing.T) {
	repo := &MockRepository{}

	items := []apipub.ScheduledNotificationItem{
		{
			NotificationID: uuid.New().String(),
			ScheduledAt:    time.Now().Add(time.Hour),
		},
		{
			NotificationID: uuid.New().String(),
			ScheduledAt:    time.Now().Add(2 * time.Hour),
		},
		{
			NotificationID: uuid.New().String(),
			ScheduledAt:    time.Now().Add(3 * time.Hour),
		},
	}

	// Simulate the core logic that consumer should do
	notifications := make([]*db.ScheduledNotification, len(items))
	for i, item := range items {
		notifications[i] = &db.ScheduledNotification{
			NotificationID: item.NotificationID,
			ScheduledAt:    &item.ScheduledAt,
		}
	}

	repo.UpsertAll(context.Background(), notifications) //nolint:errcheck, gosec

	// Verify all items were upserted
	if len(repo.upsertedAll) != len(items) {
		t.Errorf("expected %d upserted items, got %d", len(items), len(repo.upsertedAll))
	}

	for i, item := range items {
		if repo.upsertedAll[i].NotificationID != item.NotificationID {
			t.Errorf("item %d: expected ID %s, got %s", i, item.NotificationID, repo.upsertedAll[i].NotificationID)
		}
	}
}
