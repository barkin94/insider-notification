package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/ratelimit"
	"github.com/barkin/insider-notification/processor/internal/retry"
	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
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

// MessageSource is implemented by PriorityRouter.
type MessageSource interface {
	Next(ctx context.Context) (stream.Result[stream.NotificationCreatedEvent], bool)
}

// Run calls src.Next in a tight loop until ctx is cancelled or Next returns false.
func (w *Worker) Run(ctx context.Context, src MessageSource) {
	for {
		result, ok := src.Next(ctx)
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

func (w *Worker) processOne(ctx context.Context, result stream.Result[stream.NotificationCreatedEvent]) {
	evt := result.Event
	msg := result.Msg

	// deliver_after: re-enqueue and skip if not yet due
	if evt.DeliverAfter != "" {
		if t, err := time.Parse(time.RFC3339, evt.DeliverAfter); err == nil && time.Now().Before(t) {
			if err := w.pub.Publish(ctx, topicByPriority[evt.Priority], evt); err != nil {
				slog.ErrorContext(ctx, "re-enqueue deliver_after failed", "id", evt.NotificationID, "error", err)
				msg.Nack()
				return
			}
			msg.Ack()
			return
		}
	}

	// cancellation check
	cancelled, err := w.cancel.IsCancelled(ctx, evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "cancellation check error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}
	if cancelled {
		slog.InfoContext(ctx, "notification cancelled, skipping", "id", evt.NotificationID)
		msg.Ack()
		return
	}

	// processing lock
	locked, err := w.locker.TryLock(ctx, evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "lock error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}
	if !locked {
		slog.InfoContext(ctx, "lock miss, skipping", "id", evt.NotificationID)
		msg.Ack()
		return
	}
	defer w.locker.Unlock(ctx, evt.NotificationID) //nolint:errcheck

	w.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
		NotificationID: evt.NotificationID,
		Status:         model.StatusProcessing,
		AttemptNumber:  evt.AttemptNumber,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	})

	// TODO: call delivery client (notification-processing task)
	dr := delivery.Result{}

	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case dr.Success:
		w.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
			NotificationID:    evt.NotificationID,
			Status:            model.StatusDelivered,
			AttemptNumber:     evt.AttemptNumber,
			HTTPStatusCode:    dr.StatusCode,
			ProviderMessageID: dr.ProviderMsgID,
			LatencyMS:         int(dr.LatencyMS),
			UpdatedAt:         now,
		})

	case dr.Retryable && evt.AttemptNumber < evt.MaxAttempts:
		nextAttempt := evt.AttemptNumber + 1
		retryEvt := evt
		retryEvt.AttemptNumber = nextAttempt
		retryEvt.DeliverAfter = time.Now().Add(retry.Delay(nextAttempt)).UTC().Format(time.RFC3339)
		if err := w.pub.Publish(ctx, topicByPriority[evt.Priority], retryEvt); err != nil {
			slog.ErrorContext(ctx, "publish retry failed", "id", evt.NotificationID, "error", err)
		}
		w.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
			NotificationID: evt.NotificationID,
			Status:         model.StatusProcessing,
			AttemptNumber:  evt.AttemptNumber,
			HTTPStatusCode: dr.StatusCode,
			ErrorMessage:   dr.ErrorMessage,
			LatencyMS:      int(dr.LatencyMS),
			UpdatedAt:      now,
		})

	default:
		w.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
			NotificationID: evt.NotificationID,
			Status:         model.StatusFailed,
			AttemptNumber:  evt.AttemptNumber,
			HTTPStatusCode: dr.StatusCode,
			ErrorMessage:   dr.ErrorMessage,
			LatencyMS:      int(dr.LatencyMS),
			UpdatedAt:      now,
		})
	}

	msg.Ack()
}

func (w *Worker) publishStatus(ctx context.Context, evt stream.NotificationDeliveryResultEvent) {
	if err := w.pub.Publish(ctx, stream.TopicStatus, evt); err != nil {
		slog.ErrorContext(ctx, "publish status failed", "id", evt.NotificationID, "error", err)
	}
}
