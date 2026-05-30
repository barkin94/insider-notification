package delivery

import (
	"context"
	"log/slog"
	"time"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/metrics"
	"github.com/barkin/insider-notification/processor/internal/service"
	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

// MessageSource is implemented by PriorityRouter.
type MessageSource interface {
	Next(ctx context.Context) (stream.Result[stream.NotificationCreatedEvent], bool)
}

// CancellationStore checks whether a notification has been cancelled by the API.
type CancellationStore interface {
	IsCancelled(ctx context.Context, id string) (bool, error)
}

// DeliveryAttemptWriter persists and counts delivery attempt records.
type DeliveryAttemptWriter interface {
	Create(ctx context.Context, a *processordb.DeliveryAttempt) error
	CountByNotificationID(ctx context.Context, id uuid.UUID) (int, error)
}

var topicByPriority = map[string]string{
	model.PriorityHigh:   stream.TopicHigh,
	model.PriorityNormal: stream.TopicNormal,
	model.PriorityLow:    stream.TopicLow,
}

// Worker reads notification events from a MessageSource and delivers them.
type Worker struct {
	pub            stream.Publisher
	deliveryClient service.DeliveryClient
	limiter        service.Limiter
	locker         lock.Locker
	cancel         CancellationStore
	attempts       DeliveryAttemptWriter
	metrics        metrics.Metrics
}

func NewWorker(
	pub stream.Publisher,
	deliveryClient service.DeliveryClient,
	limiter service.Limiter,
	locker lock.Locker,
	cancel CancellationStore,
	attempts DeliveryAttemptWriter,
	m metrics.Metrics,
) *Worker {
	return &Worker{
		pub:            pub,
		deliveryClient: deliveryClient,
		limiter:        limiter,
		locker:         locker,
		cancel:         cancel,
		attempts:       attempts,
		metrics:        m,
	}
}

// Run calls src.Next in a tight loop until ctx is cancelled.
func (w *Worker) Run(ctx context.Context, src MessageSource) {
	for ctx.Err() == nil {
		result, ok := src.Next(ctx)
		if !ok {
			continue
		}
		if result.Err != nil {
			slog.ErrorContext(result.Ctx, "stream read error", "error", result.Err)
			continue
		}
		w.deliver(result.Ctx, result)
	}
}

// deliver runs the notification through each gate in sequence.
// Gates own their logging; deliver owns Ack/Nack.
func (w *Worker) deliver(ctx context.Context, result stream.Result[stream.NotificationCreatedEvent]) {
	ctx, span := otel.Tracer("processor").Start(ctx, "deliver")
	defer span.End()

	evt, msg := result.Event, result.Msg

	if w.isPremature(evt) {
		msg.Ack()
		return
	}

	cancelled, err := w.isCancelled(ctx, evt.NotificationID)
	if err != nil {
		msg.Nack()
		return
	}
	if cancelled {
		msg.Ack()
		return
	}

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

	limited, err := w.applyRateLimit(ctx, evt)
	if err != nil {
		msg.Nack()
		return
	}
	if limited {
		msg.Ack()
		return
	}

	notifID, err := uuid.Parse(evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "invalid notification_id", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}
	currentAttempt := w.countPriorAttempts(ctx, notifID) + 1

	dr, err := w.deliveryClient.Send(ctx, evt.Recipient, evt.Channel, evt.Content)
	if err != nil {
		slog.ErrorContext(ctx, "delivery transport error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}

	w.recordOutcome(ctx, evt, dr, notifID, currentAttempt)
	msg.Ack()
}

// countPriorAttempts returns the number of persisted delivery attempts for the
// notification. Returns 0 if the writer is nil (test convenience) or on error.
func (w *Worker) countPriorAttempts(ctx context.Context, id uuid.UUID) int {
	if w.attempts == nil {
		return 0
	}
	n, err := w.attempts.CountByNotificationID(ctx, id)
	if err != nil {
		slog.ErrorContext(ctx, "count prior attempts failed", "id", id, "error", err)
		return 0
	}
	return n
}

// isPremature reports whether the event's deliver_after has not yet passed.
func (w *Worker) isPremature(evt stream.NotificationCreatedEvent) bool {
	if evt.DeliverAfter == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, evt.DeliverAfter)
	return err == nil && time.Now().Before(t)
}

// isCancelled reports whether the notification was cancelled by the API.
func (w *Worker) isCancelled(ctx context.Context, id string) (bool, error) {
	cancelled, err := w.cancel.IsCancelled(ctx, id)
	if err != nil {
		slog.ErrorContext(ctx, "cancellation check error", "id", id, "error", err)
		return false, err
	}
	if cancelled {
		slog.InfoContext(ctx, "notification cancelled, skipping", "id", id)
	}
	return cancelled, nil
}

// applyRateLimit checks the channel's token bucket. If exhausted, it re-enqueues
// the event and returns limited=true so the caller can Ack and move on.
func (w *Worker) applyRateLimit(ctx context.Context, evt stream.NotificationCreatedEvent) (limited bool, err error) {
	allowed, err := w.limiter.Allow(ctx, evt.Channel)
	if err != nil {
		slog.ErrorContext(ctx, "rate limit error", "id", evt.NotificationID, "error", err)
		return false, err
	}
	if allowed {
		return false, nil
	}
	slog.InfoContext(ctx, "rate limited, re-enqueuing", "id", evt.NotificationID, "channel", evt.Channel)
	if err := w.pub.Publish(ctx, topicByPriority[evt.Priority], evt); err != nil {
		slog.ErrorContext(ctx, "re-enqueue rate-limited failed", "id", evt.NotificationID, "error", err)
		return false, err
	}
	return true, nil
}

// recordOutcome writes a delivery attempt when the delivery failed and, when the
// notification reaches a terminal state, publishes a status event for the API.
// On success no DB row is written — DeliveryAttempt rows exist solely to drive
// retry scheduling.
func (w *Worker) recordOutcome(ctx context.Context, evt stream.NotificationCreatedEvent, dr service.DeliveryResult, notifID uuid.UUID, currentAttempt int) {
	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case dr.Success:
		w.metrics.RecordNotificationSent(ctx, dr.LatencyMS)
		w.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
			NotificationID:    evt.NotificationID,
			Status:            model.StatusDelivered,
			AttemptNumber:     currentAttempt,
			HTTPStatusCode:    dr.StatusCode,
			ProviderMessageID: dr.ProviderMsgID,
			LatencyMS:         int(dr.LatencyMS),
			UpdatedAt:         now,
		})

	case dr.Retryable && currentAttempt < evt.MaxAttempts:
		retryAfter := time.Now().Add(service.RetryDelay(currentAttempt + 1)).UTC()
		w.writeAttempt(ctx, notifID, currentAttempt, &retryAfter, evt.Priority)

	default:
		w.metrics.RecordNotificationFailed(ctx, dr.LatencyMS)
		w.writeAttempt(ctx, notifID, currentAttempt, nil, evt.Priority)
		w.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
			NotificationID: evt.NotificationID,
			Status:         model.StatusFailed,
			AttemptNumber:  currentAttempt,
			HTTPStatusCode: dr.StatusCode,
			ErrorMessage:   dr.ErrorMessage,
			LatencyMS:      int(dr.LatencyMS),
			UpdatedAt:      now,
		})
	}
}

func (w *Worker) publishStatus(ctx context.Context, evt stream.NotificationDeliveryResultEvent) {
	if err := w.pub.Publish(ctx, stream.TopicStatus, evt); err != nil {
		slog.ErrorContext(ctx, "publish status failed", "id", evt.NotificationID, "error", err)
	}
}

func (w *Worker) writeAttempt(ctx context.Context, notifID uuid.UUID, attemptNumber int, retryAfter *time.Time, priority string) {
	if w.attempts == nil {
		return
	}
	id, err := uuid.NewV7()
	if err != nil {
		slog.ErrorContext(ctx, "generate attempt id failed", "error", err)
		return
	}
	a := &processordb.DeliveryAttempt{
		NotificationID: notifID,
		AttemptNumber:  attemptNumber,
		Priority:       priority,
		RetryAfter:     retryAfter,
	}
	a.ID = id
	if err := w.attempts.Create(ctx, a); err != nil {
		slog.ErrorContext(ctx, "write delivery attempt failed", "id", notifID, "error", err)
	}
}
