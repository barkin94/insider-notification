package consumer

import (
	"context"
	"log/slog"

	"github.com/barkin/insider-notification/processor/internal/service"
	"github.com/barkin/insider-notification/shared/stream"
)

// MessageSource is implemented by PriorityRouter.
type MessageSource interface {
	Next(ctx context.Context) (stream.Result[stream.NotificationCreatedEvent], bool)
}

// Consumer reads notification events from a MessageSource and delegates
// processing to DeliveryService.
type Consumer struct {
	svc *service.DeliveryService
}

func NewConsumer(svc *service.DeliveryService) *Consumer {
	return &Consumer{svc: svc}
}

// Run calls src.Next in a tight loop until ctx is cancelled or Next returns false.
func (c *Consumer) Run(ctx context.Context, src MessageSource) {
	for {
		result, ok := src.Next(ctx)
		if !ok {
			return
		}
		if result.Err != nil {
			slog.ErrorContext(result.Ctx, "stream read error", "error", result.Err)
			continue
		}
		c.svc.Process(result.Ctx, result)
	}
}
