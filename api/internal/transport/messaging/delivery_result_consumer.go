package messaging

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/barkin94/insider-notification/api/internal/service"
	processorpub "github.com/barkin94/insider-notification/processor/public"
	natsmsg "github.com/barkin94/insider-notification/shared/messaging/nats"
	sharedotel "github.com/barkin94/insider-notification/shared/otel"
)

// DeliveryResultConsumer processes NotificationDeliveryResultEvent messages from the status stream.
type DeliveryResultConsumer struct {
	svc  service.NotificationService
	msgs <-chan natsmsg.Result[processorpub.NotificationDeliveryResultEvent]
}

func NewDeliveryResultConsumer(svc service.NotificationService, msgs <-chan natsmsg.Result[processorpub.NotificationDeliveryResultEvent]) *DeliveryResultConsumer {
	return &DeliveryResultConsumer{svc: svc, msgs: msgs}
}

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

func (c *DeliveryResultConsumer) processOne(ctx context.Context, result natsmsg.Result[processorpub.NotificationDeliveryResultEvent]) {
	evt := result.Event

	notifID, err := uuid.Parse(evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "invalid notification_id", "notification_id", evt.NotificationID, "error", err)
		_ = result.Msg.Nak()
		sharedotel.RecordError(ctx, err)
		return
	}

	if err := c.svc.UpdateStatus(ctx, notifID, evt.Status); err != nil {
		slog.ErrorContext(ctx, "update notification status failed", "notification_id", notifID, "error", err)
		_ = result.Msg.Nak()
		sharedotel.RecordError(ctx, err)
		return
	}

	slog.InfoContext(ctx, "status event processed",
		"notification_id", notifID,
		"status", evt.Status,
		"attempt", evt.AttemptNumber,
	)
	_ = result.Msg.Ack()
}
