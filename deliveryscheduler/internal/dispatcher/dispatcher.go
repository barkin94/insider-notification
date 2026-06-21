package dispatcher

import (
	"context"
	"log/slog"
	"time"

	db "github.com/barkin/insider-notification/deliveryscheduler/internal/db"
	"github.com/barkin/insider-notification/shared/stream"
)

// ScheduledNotificationDispatcher claims due scheduled notifications and publishes them.
type ScheduledNotificationDispatcher struct {
	repo  db.ScheduledNotificationRepository
	pub   stream.Publisher
	batch int
}

func NewScheduledNotificationDispatcher(
	repo db.ScheduledNotificationRepository,
	pub stream.Publisher,
	batch int,
) *ScheduledNotificationDispatcher {
	if batch < 1 {
		batch = 100
	}
	return &ScheduledNotificationDispatcher{
		repo:  repo,
		pub:   pub,
		batch: batch,
	}
}

func (d *ScheduledNotificationDispatcher) Tick(ctx context.Context) {
	notifications, err := d.repo.DeleteByScheduledAtBeforeReturning(ctx, time.Now().UTC(), d.batch)
	if err != nil {
		slog.ErrorContext(ctx, "delivery scheduler: claim due notifications", "error", err)
		return
	}

	if len(notifications) == 0 {
		return
	}

	// Collect notification IDs
	ids := make([]string, len(notifications))
	for i, n := range notifications {
		ids[i] = n.NotificationID
	}

	// Publish as a single ScheduledNotificationDueEvent
	evt := stream.ScheduledNotificationDueEvent{
		NotificationIDs: ids,
	}
	if err := d.pub.Publish(ctx, stream.TopicScheduledNotificationDue, evt); err != nil {
		slog.ErrorContext(ctx, "delivery scheduler: publish due notifications", "count", len(ids), "error", err)
		if err := d.repo.UpsertAll(ctx, notifications); err != nil {
			slog.ErrorContext(ctx, "delivery scheduler: re-enqueue after publish failure", "count", len(notifications), "error", err)
		}
	}
}
