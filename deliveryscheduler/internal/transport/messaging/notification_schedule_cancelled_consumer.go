package messaging

import (
	"context"
	"log/slog"

	apipub "github.com/barkin94/insider-notification/api/public"
	db "github.com/barkin94/insider-notification/deliveryscheduler/internal/db"
	natsmsg "github.com/barkin94/insider-notification/shared/messaging/nats"
	sharedotel "github.com/barkin94/insider-notification/shared/otel"
)

// CancelConsumer consumes NotificationScheduleCancelledEvent and removes the
// matching row from scheduled_notifications so the dispatcher never publishes it.
type CancelConsumer struct {
	repo db.ScheduledNotificationRepository
	msgs <-chan natsmsg.Result[apipub.NotificationScheduleCancelledEvent]
}

func NewCancelConsumer(
	repo db.ScheduledNotificationRepository,
	msgs <-chan natsmsg.Result[apipub.NotificationScheduleCancelledEvent],
) *CancelConsumer {
	return &CancelConsumer{repo: repo, msgs: msgs}
}

func (c *CancelConsumer) Run(ctx context.Context) {
	natsmsg.ForEach(ctx, c.msgs, c.handleCancelEvent)
}

func (c *CancelConsumer) handleCancelEvent(result natsmsg.Result[apipub.NotificationScheduleCancelledEvent]) {
	ctx := result.Ctx
	evt := result.Event

	if err := c.repo.DeleteByNotificationID(ctx, evt.NotificationID); err != nil {
		slog.ErrorContext(ctx, "delete scheduled notification failed", "id", evt.NotificationID, "error", err)
		_ = result.Msg.Nak()
		sharedotel.RecordError(ctx, err)
		return
	}

	slog.InfoContext(ctx, "notification schedule cancelled", "id", evt.NotificationID)
	_ = result.Msg.Ack()
}
