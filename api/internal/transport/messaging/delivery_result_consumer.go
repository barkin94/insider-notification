package messaging

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/barkin94/insider-notification/api/internal/service"
	processorpub "github.com/barkin94/insider-notification/processor/public"
	stream "github.com/barkin94/insider-notification/shared/messaging"
)

// DeliveryResultConsumer processes NotificationDeliveryResultEvent messages from the status stream.
type DeliveryResultConsumer struct {
	svc  service.NotificationService
	msgs <-chan stream.Result[processorpub.NotificationDeliveryResultEvent]
}

func NewDeliveryResultConsumer(svc service.NotificationService, msgs <-chan stream.Result[processorpub.NotificationDeliveryResultEvent]) *DeliveryResultConsumer {
	return &DeliveryResultConsumer{svc: svc, msgs: msgs}
}

// Run reads from msgs until the channel is closed or ctx is cancelled.
func (c *DeliveryResultConsumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-c.msgs:
			if !ok {
				return
			}
			c.processOne(result.Ctx, result)
		}
	}
}

func (c *DeliveryResultConsumer) processOne(ctx context.Context, result stream.Result[processorpub.NotificationDeliveryResultEvent]) {
	evt := result.Event
	msg := result.Msg

	notifID, err := uuid.Parse(evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "invalid notification_id", "notification_id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}

	if err := c.svc.UpdateStatus(ctx, notifID, evt.Status); err != nil {
		slog.ErrorContext(ctx, "update notification status failed", "notification_id", notifID, "error", err)
		msg.Nack()
		return
	}

	slog.InfoContext(ctx, "status event processed",
		"notification_id", notifID,
		"status", evt.Status,
		"attempt", evt.AttemptNumber,
	)
	msg.Ack()
}
