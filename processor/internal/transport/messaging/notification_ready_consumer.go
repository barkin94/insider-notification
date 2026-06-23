package messaging

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	apipub "github.com/barkin94/insider-notification/api/public"
	"github.com/barkin94/insider-notification/processor/internal/delivery"
	natsmsg "github.com/barkin94/insider-notification/shared/messaging/nats"
	sharedotel "github.com/barkin94/insider-notification/shared/otel"
)

func (c *NotificationReadyConsumer) processOne(msg natsmsg.Result[apipub.NotificationReadyEvent]) {
	defer msg.EndSpan()
	err := c.pipeline.Run(msg.Ctx, msg.Event, msg.DeliveryCount)
	var retryAfter delivery.ErrRetryAfter
	switch {
	case errors.As(err, &retryAfter):
		_ = msg.Msg.NakWithDelay(retryAfter.Delay)
		slog.InfoContext(msg.Ctx, "message nacked with delay", "id", msg.Event.NotificationID, "delay", retryAfter.Delay)
	case err != nil:
		_ = msg.Msg.Nak()
		slog.ErrorContext(msg.Ctx, "pipeline error", "id", msg.Event.NotificationID, "error", err)
		sharedotel.RecordError(msg.Ctx, err)
	default:
		_ = msg.Msg.Ack()
	}
}

// NotificationReadyConsumer subscribes to the three NATS JetStream priority subjects
// and fans work out to a fixed-size worker pool. On a retryable result it calls
// NakWithDelay so NATS re-delivers the same message after the backoff period,
// eliminating the need for a separate retry scheduler service.
type NotificationReadyConsumer struct {
	router      *delivery.PriorityRouter[natsmsg.Result[apipub.NotificationReadyEvent]]
	pipeline    *delivery.NotificationDeliveryPipeline
	concurrency int
}

// NewNotificationReadyConsumer subscribes to high/normal/low NATS subjects and
// returns a consumer ready to call StartMessageProcessing.
// maxDeliver is passed to NATS as the per-consumer MaxDeliver limit (safety net
// above the application-level MaxAttempts).
func NewNotificationReadyConsumer(
	ctx context.Context,
	h *natsmsg.Handle,
	serviceName string,
	highWeight, normalWeight, lowWeight int,
	pipeline *delivery.NotificationDeliveryPipeline,
	concurrency int,
	maxDeliver int,
) *NotificationReadyConsumer {
	highMsgs := natsmsg.Subscribe[apipub.NotificationReadyEvent](
		ctx, h, string(apipub.TopicHigh), "processor-high", serviceName, maxDeliver,
	)
	normalMsgs := natsmsg.Subscribe[apipub.NotificationReadyEvent](
		ctx, h, string(apipub.TopicNormal), "processor-normal", serviceName, maxDeliver,
	)
	lowMsgs := natsmsg.Subscribe[apipub.NotificationReadyEvent](
		ctx, h, string(apipub.TopicLow), "processor-low", serviceName, maxDeliver,
	)

	router := delivery.NewPriorityRouter([]delivery.WeightedSource[natsmsg.Result[apipub.NotificationReadyEvent]]{
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
					if ctx.Err() != nil {
						return
					}
					continue
				}
				c.processOne(msg)
			}
		}()
	}
	wg.Wait()
}
