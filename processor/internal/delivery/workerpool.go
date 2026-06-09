package delivery

import (
	"context"
	"log/slog"
	"sync"

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
				nextNtfnChannelToProcess, ok := c.notificationSelectorByPriority.Next(ctx)
				if !ok {
					continue
				}
				if err := c.notificationDeliveryPipeline.Run(nextNtfnChannelToProcess.Ctx, nextNtfnChannelToProcess); err != nil {
					slog.ErrorContext(nextNtfnChannelToProcess.Ctx, "pipeline error", "error", err)
				}
			}
		}()
	}
	wg.Wait()
}
