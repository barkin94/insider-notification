package dispatcher

import (
	"context"
	"log/slog"
	"time"

	db "github.com/barkin94/insider-notification/deliveryscheduler/internal/db"
	dspub "github.com/barkin94/insider-notification/deliveryscheduler/public"
	stream "github.com/barkin94/insider-notification/shared/messaging"
	sharedotel "github.com/barkin94/insider-notification/shared/otel"
)

// ScheduledNotificationDispatcher claims due scheduled notifications and publishes them.
type ScheduledNotificationDispatcher struct {
	repo  db.ScheduledNotificationRepository
	pub   stream.BatchPublisher
	batch int
}

func NewScheduledNotificationDispatcher(
	repo db.ScheduledNotificationRepository,
	pub stream.BatchPublisher,
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
	for {
		notifications, err := d.repo.DeleteByScheduledAtBeforeReturning(ctx, time.Now().UTC(), d.batch)
		if err != nil {
			slog.ErrorContext(ctx, "delivery scheduler: claim due notifications", "error", err)
			return
		}
		if len(notifications) == 0 {
			return
		}

		msgs := make([]stream.BatchMessage, len(notifications))
		for i, n := range notifications {
			msgs[i] = stream.BatchMessage{
				Ctx:     sharedotel.ContextWithTraceMetadata(ctx, n.TraceMetadata),
				Payload: dspub.ScheduledNotificationDueEvent{NotificationID: n.NotificationID},
			}
		}

		if err := d.pub.PublishBatch(ctx, dspub.TopicScheduledNotificationDue, msgs); err != nil {
			slog.ErrorContext(ctx, "delivery scheduler: publish due notifications", "count", len(msgs), "error", err)
			if err := d.repo.UpsertAll(ctx, notifications); err != nil {
				slog.ErrorContext(ctx, "delivery scheduler: re-enqueue after publish failure", "count", len(notifications), "error", err)
			}
			return
		}
	}
}
