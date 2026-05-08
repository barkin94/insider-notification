package worker

import (
	"context"
	"log/slog"

	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/ratelimit"
)

// StreamPublisher publishes events to a stream topic.
type StreamPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

// CancellationStore checks whether a notification has been cancelled by the API.
type CancellationStore interface {
	IsCancelled(ctx context.Context, id string) (bool, error)
}

var topicByPriority = map[string]string{
	model.PriorityHigh:   stream.TopicHigh,
	model.PriorityNormal: stream.TopicNormal,
	model.PriorityLow:    stream.TopicLow,
}

// Worker processes notification delivery events from a stream.
type Worker struct {
	pub      StreamPublisher
	delivery delivery.Client
	limiter  ratelimit.Limiter
	locker   lock.Locker
	cancel   CancellationStore
}

func NewWorker(
	pub StreamPublisher,
	delivery delivery.Client,
	limiter ratelimit.Limiter,
	locker lock.Locker,
	cancel CancellationStore,
) *Worker {
	return &Worker{
		pub:      pub,
		delivery: delivery,
		limiter:  limiter,
		locker:   locker,
		cancel:   cancel,
	}
}

// Run reads from msgs until the channel is closed or ctx is cancelled.
func (w *Worker) Run(ctx context.Context, msgs <-chan stream.Result[stream.NotificationCreatedEvent]) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-msgs:
			if !ok {
				return
			}
			if result.Err != nil {
				slog.ErrorContext(ctx, "stream read error", "error", result.Err)
				result.Msg.Nack()
				continue
			}
			w.processOne(ctx, result)
		}
	}
}

// processOne drives a single notification through the delivery pipeline.
// TODO: implement full delivery logic (see specs/agent/tasks/notification-processing.md)
func (w *Worker) processOne(ctx context.Context, result stream.Result[stream.NotificationCreatedEvent]) {
	evt := result.Event
	msg := result.Msg

	locked, err := w.locker.TryLock(ctx, evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "lock error", "notification_id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}
	if !locked {
		slog.InfoContext(ctx, "lock miss, skipping", "notification_id", evt.NotificationID)
		msg.Ack()
		return
	}
	defer w.locker.Unlock(ctx, evt.NotificationID) //nolint:errcheck

	msg.Ack()
}
