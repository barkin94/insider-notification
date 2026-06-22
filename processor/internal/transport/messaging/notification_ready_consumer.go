package messaging

import (
	"context"
	"log/slog"
	"sync"

	"github.com/ThreeDotsLabs/watermill/message"

	apipub "github.com/barkin94/insider-notification/api/public"
	"github.com/barkin94/insider-notification/processor/internal/delivery"
	stream "github.com/barkin94/insider-notification/shared/messaging"
	sharedotel "github.com/barkin94/insider-notification/shared/otel"
)

type NotificationReadyConsumer struct {
	router      *delivery.PriorityRouter[stream.Result[apipub.NotificationReadyEvent]]
	pipeline    *delivery.NotificationDeliveryPipeline
	concurrency int
}

func NewNotificationReadyConsumer(
	ctx context.Context,
	sub message.Subscriber,
	serviceName string,
	highWeight, normalWeight, lowWeight int,
	pipeline *delivery.NotificationDeliveryPipeline,
	concurrency int,
) *NotificationReadyConsumer {
	highMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, string(apipub.TopicHigh), serviceName)
	normalMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, string(apipub.TopicNormal), serviceName)
	lowMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, string(apipub.TopicLow), serviceName)

	router := delivery.NewPriorityRouter([]delivery.WeightedSource[stream.Result[apipub.NotificationReadyEvent]]{
		{Ch: highMsgs, Weight: highWeight},
		{Ch: normalMsgs, Weight: normalWeight},
		{Ch: lowMsgs, Weight: lowWeight},
	})

	return &NotificationReadyConsumer{router: router, pipeline: pipeline, concurrency: concurrency}
}

func (c *NotificationReadyConsumer) StartMessageProcessing(ctx context.Context) {
	var wg sync.WaitGroup
	for range c.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				msg, ok := c.router.Next(ctx)
				if !ok {
					continue
				}
				if err := c.pipeline.Run(msg.Ctx, msg); err != nil {
					slog.ErrorContext(msg.Ctx, "pipeline error", "error", err)
					sharedotel.RecordError(msg.Ctx, err)
				}
			}
		}()
	}
	wg.Wait()
}
