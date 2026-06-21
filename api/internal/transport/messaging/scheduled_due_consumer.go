package messaging

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/barkin/insider-notification/api/internal/repository"
	stream "github.com/barkin/insider-notification/shared/messaging"
)

// NotificationReader is the narrow read port for fetching notifications.
type NotificationReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*repository.Notification, error)
}

// ScheduledDueConsumer consumes ScheduledNotificationDueEvent and publishes
// the full notification details as NotificationReadyEvent to the processor.
type ScheduledDueConsumer struct {
	repo      NotificationReader
	publisher stream.Publisher
	msgs      <-chan stream.Result[stream.ScheduledNotificationDueEvent]
}

func NewScheduledDueConsumer(
	repo NotificationReader,
	publisher stream.Publisher,
	msgs <-chan stream.Result[stream.ScheduledNotificationDueEvent],
) *ScheduledDueConsumer {
	return &ScheduledDueConsumer{repo: repo, publisher: publisher, msgs: msgs}
}

func (c *ScheduledDueConsumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-c.msgs:
			if !ok {
				return
			}
			c.handleScheduledDueEvent(result.Ctx, result)
		}
	}
}

func (c *ScheduledDueConsumer) handleScheduledDueEvent(ctx context.Context, result stream.Result[stream.ScheduledNotificationDueEvent]) {
	evt := result.Event
	msg := result.Msg

	topicByPriority := map[string]string{
		"high":   stream.TopicHigh,
		"normal": stream.TopicNormal,
		"low":    stream.TopicLow,
	}

	for _, notifID := range evt.NotificationIDs {
		id, err := uuid.Parse(notifID)
		if err != nil {
			slog.ErrorContext(ctx, "scheduled due consumer: parse notification id", "id", notifID, "error", err)
			msg.Nack()
			return
		}

		notif, err := c.repo.GetByID(ctx, id)
		if err != nil {
			slog.ErrorContext(ctx, "scheduled due consumer: fetch notification", "id", notifID, "error", err)
			msg.Nack()
			return
		}

		readyEvt := stream.NotificationReadyEvent{
			NotificationID: notif.ID.String(),
			Channel:        notif.Channel,
			Recipient:      notif.Recipient,
			Content:        notif.Content,
			Priority:       notif.Priority,
			MaxAttempts:    notif.MaxAttempts,
		}

		topic := topicByPriority[notif.Priority]
		if err := c.publisher.Publish(ctx, topic, readyEvt); err != nil {
			slog.ErrorContext(ctx, "scheduled due consumer: publish notification ready", "id", notifID, "error", err)
			msg.Nack()
			return
		}

		slog.InfoContext(ctx, "scheduled notification dispatched", "id", notifID)
	}

	msg.Ack()
}
