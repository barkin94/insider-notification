package messaging

import (
	"context"
	"log/slog"

	apipub "github.com/barkin94/insider-notification/api/public"
	db "github.com/barkin94/insider-notification/deliveryscheduler/internal/db"
	stream "github.com/barkin94/insider-notification/shared/messaging"
)

// CancelConsumer consumes NotificationScheduleCancelledEvent and removes the
// matching row from scheduled_notifications so the dispatcher never publishes it.
type CancelConsumer struct {
	repo db.ScheduledNotificationRepository
	msgs <-chan stream.Result[apipub.NotificationScheduleCancelledEvent]
}

func NewCancelConsumer(
	repo db.ScheduledNotificationRepository,
	msgs <-chan stream.Result[apipub.NotificationScheduleCancelledEvent],
) *CancelConsumer {
	return &CancelConsumer{repo: repo, msgs: msgs}
}

func (c *CancelConsumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-c.msgs:
			if !ok {
				return
			}
			c.handleCancelEvent(result.Ctx, result)
		}
	}
}

func (c *CancelConsumer) handleCancelEvent(ctx context.Context, result stream.Result[apipub.NotificationScheduleCancelledEvent]) {
	evt := result.Event
	msg := result.Msg

	if err := c.repo.DeleteByNotificationID(ctx, evt.NotificationID); err != nil {
		slog.ErrorContext(ctx, "delete scheduled notification failed", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}

	slog.InfoContext(ctx, "notification schedule cancelled", "id", evt.NotificationID)
	msg.Ack()
}
