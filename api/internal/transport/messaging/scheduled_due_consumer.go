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
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*db.Notification, error)
}

// ScheduledDueConsumer consumes ScheduledNotificationDueEvent batches, bulk-fetches
// the full notification details in a single query, and publishes NotificationReadyEvent
// to the processor for each one.
type ScheduledDueConsumer struct {
	repo      NotificationReader
	publisher stream.Publisher
	msgs      <-chan []natsmsg.Result[dspub.ScheduledNotificationDueEvent]
}

func NewScheduledDueConsumer(
	repo NotificationReader,
	publisher stream.Publisher,
	msgs <-chan []natsmsg.Result[dspub.ScheduledNotificationDueEvent],
) *ScheduledDueConsumer {
	return &ScheduledDueConsumer{repo: repo, publisher: publisher, msgs: msgs}
}

func (c *ScheduledDueConsumer) Run(ctx context.Context) {
	natsmsg.ForEachBatch(ctx, c.msgs, c.handleBatch)
}

func (c *ScheduledDueConsumer) handleBatch(batch []natsmsg.Result[dspub.ScheduledNotificationDueEvent]) {
	// Parse IDs and map each uuid back to its Result for per-message ACK/NAK.
	ids := make([]uuid.UUID, 0, len(batch))
	resultByID := make(map[uuid.UUID]natsmsg.Result[dspub.ScheduledNotificationDueEvent], len(batch))

	for _, result := range batch {
		id, err := uuid.Parse(result.Event.NotificationID)
		if err != nil {
			slog.ErrorContext(result.Ctx, "scheduled due consumer: parse notification id", "id", result.Event.NotificationID, "error", err)
			sharedotel.RecordError(result.Ctx, err)
			_ = result.Msg.Nak()
			continue
		}
		ids = append(ids, id)
		resultByID[id] = result
	}

	if len(ids) == 0 {
		return
	}

	// Single bulk DB fetch for the whole batch.
	notifs, err := c.repo.GetByIDs(batch[0].Ctx, ids)
	if err != nil {
		slog.ErrorContext(batch[0].Ctx, "scheduled due consumer: bulk fetch notifications", "count", len(ids), "notification_ids", ids, "error", err)
		for _, result := range resultByID {
			sharedotel.RecordError(result.Ctx, err)
			_ = result.Msg.Nak()
		}
		return
	}

	notifByID := make(map[uuid.UUID]*db.Notification, len(notifs))
	for _, n := range notifs {
		notifByID[n.ID] = n
	}

	for id, result := range resultByID {
		notif, ok := notifByID[id]
		if !ok {
			slog.ErrorContext(result.Ctx, "scheduled due consumer: notification not found", "id", id)
			_ = result.Msg.Nak()
			continue
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
		if err := c.publisher.Publish(result.Ctx, string(topic), readyEvt); err != nil {
			slog.ErrorContext(result.Ctx, "scheduled due consumer: publish notification ready", "id", id, "error", err)
			sharedotel.RecordError(result.Ctx, err)
			_ = result.Msg.Nak()
			continue
		}

		slog.InfoContext(result.Ctx, "scheduled notification dispatched", "id", id)
		_ = result.Msg.Ack()
	}
}
