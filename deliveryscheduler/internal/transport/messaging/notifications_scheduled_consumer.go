package messaging

import (
	"context"
	"log/slog"

	apipub "github.com/barkin94/insider-notification/api/public"
	db "github.com/barkin94/insider-notification/deliveryscheduler/internal/db"
	sharedbun "github.com/barkin94/insider-notification/shared/bun"
	natsmsg "github.com/barkin94/insider-notification/shared/messaging/nats"
	sharedotel "github.com/barkin94/insider-notification/shared/otel"
)

// Consumer consumes NotificationsScheduledEvent and persists scheduled notifications to Postgres
// so the dispatcher picks them up when scheduled_at time has passed.
type Consumer struct {
	repo db.ScheduledNotificationRepository
	msgs <-chan natsmsg.Result[apipub.NotificationsScheduledEvent]
}

func NewConsumer(
	repo db.ScheduledNotificationRepository,
	msgs <-chan natsmsg.Result[apipub.NotificationsScheduledEvent],
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

func (c *Consumer) handleScheduledEvents(ctx context.Context, result natsmsg.Result[apipub.NotificationsScheduledEvent]) {
	evt := result.Event

	traceMetadata := sharedotel.ExtractTraceMetadata(ctx)
	notifications := make([]*db.ScheduledNotification, len(evt.Notifications))
	for i, item := range evt.Notifications {
		scheduledAt := item.ScheduledAt
		notifications[i] = &db.ScheduledNotification{
			NotificationID:     item.NotificationID,
			ScheduledAt:        &scheduledAt,
			TraceMetadataModel: sharedbun.TraceMetadataModel{TraceMetadata: traceMetadata},
		}
	}

	if err := c.repo.UpsertAll(ctx, notifications); err != nil {
		slog.ErrorContext(ctx, "persist scheduled notifications failed", "count", len(notifications), "error", err)
		_ = result.Msg.Nak()
		sharedotel.RecordError(ctx, err)
		return
	}

	for _, item := range evt.Notifications {
		slog.InfoContext(ctx, "notification scheduled", "id", item.NotificationID, "scheduled_at", item.ScheduledAt)
	}
	_ = result.Msg.Ack()
}
