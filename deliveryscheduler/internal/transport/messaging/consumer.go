package messaging

import (
	"context"
	"log/slog"

	apipub "github.com/barkin94/insider-notification/api/public"
	db "github.com/barkin94/insider-notification/deliveryscheduler/internal/db"
	stream "github.com/barkin94/insider-notification/shared/messaging"
)

// Consumer consumes NotificationsScheduledEvent and persists scheduled notifications to Postgres
// so the dispatcher picks them up when scheduled_at time has passed.
type Consumer struct {
	repo db.ScheduledNotificationRepository
	msgs <-chan stream.Result[apipub.NotificationsScheduledEvent]
}

func NewConsumer(
	repo db.ScheduledNotificationRepository,
	msgs <-chan stream.Result[apipub.NotificationsScheduledEvent],
) *Consumer {
	return &Consumer{repo: repo, msgs: msgs}
}

func (c *Consumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-c.msgs:
			if !ok {
				return
			}
			c.handleScheduledEvents(result.Ctx, result)
		}
	}
}

func (c *Consumer) handleScheduledEvents(ctx context.Context, result stream.Result[apipub.NotificationsScheduledEvent]) {
	evt := result.Event
	msg := result.Msg

	notifications := make([]*db.ScheduledNotification, len(evt.Notifications))
	for i, item := range evt.Notifications {
		notifications[i] = &db.ScheduledNotification{
			NotificationID: item.NotificationID,
			ScheduledAt:    &item.ScheduledAt,
		}
	}

	if err := c.repo.UpsertAll(ctx, notifications); err != nil {
		slog.ErrorContext(ctx, "persist scheduled notifications failed", "count", len(notifications), "error", err)
		msg.Nack()
		return
	}

	for _, item := range evt.Notifications {
		slog.InfoContext(ctx, "notification scheduled", "id", item.NotificationID, "scheduled_at", item.ScheduledAt)
	}
	msg.Ack()
}
