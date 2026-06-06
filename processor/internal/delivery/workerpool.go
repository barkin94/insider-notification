package delivery

import (
	"context"
	"log/slog"
	"sync"

	"github.com/barkin/insider-notification/shared/stream"
)

// WorkerPool fans out stream messages from the router to N concurrent pipeline workers.
type NotificationDeliveryWorkerPool struct {
	notificationSelector         *PriorityRouter[stream.Result[stream.NotificationReadyEvent]]
	notificationDeliveryPipeline *NotificationDeliveryPipeline
	concurrency                  int
}

func NewNotificationDeliveryWorkerPool(
	notificationSelector *PriorityRouter[stream.Result[stream.NotificationReadyEvent]],
	notificationDeliveryPipeline *NotificationDeliveryPipeline,
	concurrency int,
) *NotificationDeliveryWorkerPool {
	return &NotificationDeliveryWorkerPool{notificationSelector: notificationSelector, notificationDeliveryPipeline: notificationDeliveryPipeline, concurrency: concurrency}
}

func (c *NotificationDeliveryWorkerPool) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range c.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				result, ok := c.notificationSelector.Next(ctx)
				if !ok {
					continue
				}
				if result.Err != nil {
					slog.ErrorContext(result.Ctx, "stream read error", "error", result.Err)
					continue
				}
				c.notificationDeliveryPipeline.Run(result.Ctx, result)
			}
		}()
	}
	wg.Wait()
}
