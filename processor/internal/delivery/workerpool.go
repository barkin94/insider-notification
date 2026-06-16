package delivery

import (
	"context"
	"log/slog"
	"sync"

	sharedotel "github.com/barkin/insider-notification/shared/otel"
	"github.com/barkin/insider-notification/shared/stream"
)

// WorkerPool fans out stream messages from the router to N concurrent pipeline workers.
type NotificationDeliveryWorkerPool struct {
	notificationSelectorByPriority *PriorityRouter[stream.Result[stream.NotificationReadyEvent]]
	notificationDeliveryPipeline   *NotificationDeliveryPipeline
	concurrency                    int
}

func NewNotificationDeliveryWorkerPool(
	notificationSelector *PriorityRouter[stream.Result[stream.NotificationReadyEvent]],
	notificationDeliveryPipeline *NotificationDeliveryPipeline,
	concurrency int,
) *NotificationDeliveryWorkerPool {
	return &NotificationDeliveryWorkerPool{notificationSelectorByPriority: notificationSelector, notificationDeliveryPipeline: notificationDeliveryPipeline, concurrency: concurrency}
}

func (c *NotificationDeliveryWorkerPool) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range c.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				ntfnChannelToProcess, ok := c.notificationSelectorByPriority.Next(ctx)
				if !ok {
					continue
				}
				if err := c.notificationDeliveryPipeline.Run(ntfnChannelToProcess.Ctx, ntfnChannelToProcess); err != nil {
					slog.ErrorContext(ntfnChannelToProcess.Ctx, "pipeline error", "error", err)
					sharedotel.RecordError(ntfnChannelToProcess.Ctx, err)
				}
			}
		}()
	}
	wg.Wait()
}
