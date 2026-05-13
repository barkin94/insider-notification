package worker

import (
	"context"
	"log/slog"
	"time"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/worker/ratelimit"
	"github.com/barkin/insider-notification/processor/internal/worker/retry"
	"github.com/barkin/insider-notification/processor/internal/worker/webhook"
	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

// StreamPublisher publishes events to a stream topic.
type StreamPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

// CancellationStore checks whether a notification has been cancelled by the API.
type CancellationStore interface {
	IsCancelled(ctx context.Context, id string) (bool, error)
}

// DeliveryAttemptWriter persists a delivery attempt record.
type DeliveryAttemptWriter interface {
	Create(ctx context.Context, a *processordb.DeliveryAttempt) error
}

var topicByPriority = map[string]string{
	model.PriorityHigh:   stream.TopicHigh,
	model.PriorityNormal: stream.TopicNormal,
	model.PriorityLow:    stream.TopicLow,
}

// Worker processes notification delivery events from a stream.
type Worker struct {
	pub           StreamPublisher
	webhookClient webhook.Client
	limiter       ratelimit.Limiter
	locker        lock.Locker
	cancel        CancellationStore
	attempts      DeliveryAttemptWriter
}

func NewWorker(
	pub StreamPublisher,
	webhookClient webhook.Client,
	limiter ratelimit.Limiter,
	locker lock.Locker,
	cancel CancellationStore,
	attempts DeliveryAttemptWriter,
) *Worker {
	return &Worker{
		pub:           pub,
		webhookClient: webhookClient,
		limiter:       limiter,
		locker:        locker,
		cancel:        cancel,
		attempts:      attempts,
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
			slog.ErrorContext(result.Ctx, "stream read error", "error", result.Err)
			continue
		}
		w.processOne(result.Ctx, result)
	}
}

func (w *Worker) processOne(ctx context.Context, result stream.Result[stream.NotificationCreatedEvent]) {
	ctx, span := otel.Tracer("processor").Start(ctx, "processOne")
	defer span.End()

	evt := result.Event
	msg := result.Msg

	// deliver_after: drop if not yet due — scheduler re-publishes when ready
	if evt.DeliverAfter != "" {
		if t, err := time.Parse(time.RFC3339, evt.DeliverAfter); err == nil && time.Now().Before(t) {
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

	// rate limit
	allowed, err := w.limiter.Allow(ctx, evt.Channel)
	if err != nil {
		slog.ErrorContext(ctx, "rate limit error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}
	if !allowed {
		slog.InfoContext(ctx, "rate limited, re-enqueuing", "id", evt.NotificationID, "channel", evt.Channel)
		if err := w.pub.Publish(ctx, topicByPriority[evt.Priority], evt); err != nil {
			slog.ErrorContext(ctx, "re-enqueue rate-limited failed", "id", evt.NotificationID, "error", err)
			msg.Nack()
			return
		}
		msg.Ack()
		return
	}

	dr, err := w.webhookClient.Send(ctx, evt.Recipient, evt.Channel, evt.Content)
	if err != nil {
		slog.ErrorContext(ctx, "delivery transport error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case dr.Success:
		w.writeAttempt(ctx, evt.NotificationID, evt.AttemptNumber, model.StatusDelivered, nil, evt.Priority)
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
		retryAfter := time.Now().Add(retry.Delay(evt.AttemptNumber + 1)).UTC()
		w.writeAttempt(ctx, evt.NotificationID, evt.AttemptNumber, model.StatusFailed, &retryAfter, evt.Priority)
	default:
		w.writeAttempt(ctx, evt.NotificationID, evt.AttemptNumber, model.StatusFailed, nil, evt.Priority)
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

func (w *Worker) writeAttempt(ctx context.Context, notifIDStr string, attemptNumber int, status string, retryAfter *time.Time, priority string) {
	if w.attempts == nil {
		return
	}
	notifID, err := uuid.Parse(notifIDStr)
	if err != nil {
		slog.ErrorContext(ctx, "invalid notification_id for attempt write", "id", notifIDStr, "error", err)
		return
	}
	id, err := uuid.NewV7()
	if err != nil {
		slog.ErrorContext(ctx, "generate attempt id failed", "error", err)
		return
	}
	now := time.Now().UTC()
	a := &processordb.DeliveryAttempt{
		NotificationID: notifID,
		AttemptNumber:  attemptNumber,
		Status:         status,
		Priority:       priority,
		RetryAfter:     retryAfter,
	}
	a.ID = id
	a.CreatedAt = now
	a.UpdatedAt = now
	if err := w.attempts.Create(ctx, a); err != nil {
		slog.ErrorContext(ctx, "write delivery attempt failed", "id", notifIDStr, "error", err)
	}
}
