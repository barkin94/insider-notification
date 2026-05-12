package consumer

import (
	"context"
	"log/slog"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

// StatusConsumer processes NotificationDeliveryResultEvent messages from the status stream.
type StatusConsumer struct {
	notifRepo db.NotificationRepository
}

func NewStatusConsumer(notifRepo db.NotificationRepository) *StatusConsumer {
	return &StatusConsumer{notifRepo: notifRepo}
}

// Run reads from msgs until the channel is closed or ctx is cancelled.
func (c *StatusConsumer) Run(ctx context.Context, msgs <-chan stream.Result[stream.NotificationDeliveryResultEvent]) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-msgs:
			if !ok {
				return
			}
			if result.Err != nil {
				slog.ErrorContext(result.Ctx, "status stream read error", "error", result.Err)
				continue
			}
			c.processOne(result.Ctx, result)
		}
	}
}

func (c *StatusConsumer) processOne(ctx context.Context, result stream.Result[stream.NotificationDeliveryResultEvent]) {
	ctx, span := otel.Tracer("api").Start(ctx, "statusConsumer.processOne")
	defer span.End()

	evt := result.Event
	msg := result.Msg

	notifID, err := uuid.Parse(evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "invalid notification_id", "notification_id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}

	if err := c.notifRepo.UpdateStatus(ctx, notifID, evt.Status); err != nil {
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
