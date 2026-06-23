package messaging

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/barkin94/insider-notification/api/internal/db"
	apipub "github.com/barkin94/insider-notification/api/public"
	dspub "github.com/barkin94/insider-notification/deliveryscheduler/public"
	stream "github.com/barkin94/insider-notification/shared/messaging"
	natsmsg "github.com/barkin94/insider-notification/shared/messaging/nats"
	sharedotel "github.com/barkin94/insider-notification/shared/otel"
)

// NotificationReader is the narrow read port for fetching notifications.
type NotificationReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*db.Notification, error)
}

// ScheduledDueConsumer consumes ScheduledNotificationDueEvent and publishes
// the full notification details as NotificationReadyEvent to the processor.
type ScheduledDueConsumer struct {
	repo      NotificationReader
	publisher stream.Publisher
	msgs      <-chan natsmsg.Result[dspub.ScheduledNotificationDueEvent]
}

func NewScheduledDueConsumer(
	repo NotificationReader,
	publisher stream.Publisher,
	msgs <-chan natsmsg.Result[dspub.ScheduledNotificationDueEvent],
) *ScheduledDueConsumer {
	return &ScheduledDueConsumer{repo: repo, publisher: publisher, msgs: msgs}
}

func (c *ScheduledDueConsumer) Run(ctx context.Context) {
	natsmsg.ForEach(ctx, c.msgs, c.handleScheduledDueEvent)
}

func (c *ScheduledDueConsumer) handleScheduledDueEvent(result natsmsg.Result[dspub.ScheduledNotificationDueEvent]) {
	ctx := result.Ctx
	evt := result.Event

	for _, notifID := range evt.NotificationIDs {
		id, err := uuid.Parse(notifID)
		if err != nil {
			slog.ErrorContext(ctx, "scheduled due consumer: parse notification id", "id", notifID, "error", err)
			_ = result.Msg.Nak()
			sharedotel.RecordError(ctx, err)
			return
		}

		notif, err := c.repo.GetByID(ctx, id)
		if err != nil {
			slog.ErrorContext(ctx, "scheduled due consumer: fetch notification", "id", notifID, "error", err)
			_ = result.Msg.Nak()
			sharedotel.RecordError(ctx, err)
			return
		}

		readyEvt := apipub.NotificationReadyEvent{
			NotificationID: notif.ID.String(),
			Channel:        notif.Channel,
			Recipient:      notif.Recipient,
			Content:        notif.Content,
			Priority:       notif.Priority,
			MaxAttempts:    notif.MaxAttempts,
		}

		topic := apipub.TopicByPriority[apipub.Priority(notif.Priority)]
		if err := c.publisher.Publish(ctx, string(topic), readyEvt); err != nil {
			slog.ErrorContext(ctx, "scheduled due consumer: publish notification ready", "id", notifID, "error", err)
			_ = result.Msg.Nak()
			sharedotel.RecordError(ctx, err)
			return
		}

		slog.InfoContext(ctx, "scheduled notification dispatched", "id", notifID)
	}

	_ = result.Msg.Ack()
}
