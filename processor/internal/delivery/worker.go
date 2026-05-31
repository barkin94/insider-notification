package delivery

import (
	"context"
	"log/slog"
	"time"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/service"
	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

var topicByPriority = map[string]string{
	model.PriorityHigh:   stream.TopicHigh,
	model.PriorityNormal: stream.TopicNormal,
	model.PriorityLow:    stream.TopicLow,
}

// NotificationDeliveryPipeline runs a single notification event through each
// gate in sequence: premature check, cancellation, locking, rate limiting,
// delivery, and outcome recording.
type NotificationDeliveryPipeline struct {
	pub            stream.Publisher
	deliveryClient service.NtfnDeliveryClient
	limiter        service.Limiter
	locker         lock.Locker
	cancel         service.CancellationStore
	attempts       processordb.DeliveryAttemptRepository
	metrics        service.Metrics
}

func NewNotificationDeliveryPipeline(
	pub stream.Publisher,
	deliveryClient service.NtfnDeliveryClient,
	limiter service.Limiter,
	locker lock.Locker,
	cancel service.CancellationStore,
	attempts processordb.DeliveryAttemptRepository,
	m service.Metrics,
) *NotificationDeliveryPipeline {
	return &NotificationDeliveryPipeline{
		pub:            pub,
		deliveryClient: deliveryClient,
		limiter:        limiter,
		locker:         locker,
		cancel:         cancel,
		attempts:       attempts,
		metrics:        m,
	}
}

// Run runs the notification through each gate in sequence.
// Gates own their logging; Run owns Ack/Nack.
func (p *NotificationDeliveryPipeline) Run(ctx context.Context, result stream.Result[stream.NotificationCreatedEvent]) {
	ctx, span := otel.Tracer("processor").Start(ctx, "deliver")
	defer span.End()

	evt, msg := result.Event, result.Msg

	if p.isPremature(evt) {
		msg.Ack()
		return
	}

	cancelled, err := p.isCancelled(ctx, evt.NotificationID)
	if err != nil {
		msg.Nack()
		return
	}
	if cancelled {
		msg.Ack()
		return
	}

	locked, err := p.locker.TryLock(ctx, evt.NotificationID)
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
	defer p.locker.Unlock(ctx, evt.NotificationID) //nolint:errcheck

	limited, err := p.applyRateLimit(ctx, evt)
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
	currentAttempt := p.countPriorAttempts(ctx, notifID) + 1

	dr, err := p.deliveryClient.Send(ctx, evt.Recipient, evt.Channel, evt.Content)
	if err != nil {
		slog.ErrorContext(ctx, "delivery transport error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}

	p.recordOutcome(ctx, evt, dr, notifID, currentAttempt)
	msg.Ack()
}

// countPriorAttempts returns the number of persisted delivery attempts for the
// notification. Returns 0 if the writer is nil (test convenience) or on error.
func (p *NotificationDeliveryPipeline) countPriorAttempts(ctx context.Context, id uuid.UUID) int {
	if p.attempts == nil {
		return 0
	}
	n, err := p.attempts.CountByNotificationID(ctx, id)
	if err != nil {
		slog.ErrorContext(ctx, "count prior attempts failed", "id", id, "error", err)
		return 0
	}
	return n
}

// isPremature reports whether the event's deliver_after has not yet passed.
func (p *NotificationDeliveryPipeline) isPremature(evt stream.NotificationCreatedEvent) bool {
	if evt.DeliverAfter == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, evt.DeliverAfter)
	return err == nil && time.Now().Before(t)
}

// isCancelled reports whether the notification was cancelled by the API.
func (p *NotificationDeliveryPipeline) isCancelled(ctx context.Context, id string) (bool, error) {
	cancelled, err := p.cancel.IsCancelled(ctx, id)
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
func (p *NotificationDeliveryPipeline) applyRateLimit(ctx context.Context, evt stream.NotificationCreatedEvent) (limited bool, err error) {
	allowed, err := p.limiter.Allow(ctx, evt.Channel)
	if err != nil {
		slog.ErrorContext(ctx, "rate limit error", "id", evt.NotificationID, "error", err)
		return false, err
	}
	if allowed {
		return false, nil
	}
	slog.InfoContext(ctx, "rate limited, re-enqueuing", "id", evt.NotificationID, "channel", evt.Channel)
	if err := p.pub.Publish(ctx, topicByPriority[evt.Priority], evt); err != nil {
		slog.ErrorContext(ctx, "re-enqueue rate-limited failed", "id", evt.NotificationID, "error", err)
		return false, err
	}
	return true, nil
}

// recordOutcome writes a delivery attempt when the delivery failed and, when the
// notification reaches a terminal state, publishes a status event for the API.
// On success no DB row is written — DeliveryAttempt rows exist solely to drive
// retry scheduling.
func (p *NotificationDeliveryPipeline) recordOutcome(ctx context.Context, evt stream.NotificationCreatedEvent, dr service.DeliveryResult, notifID uuid.UUID, currentAttempt int) {
	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case dr.Success:
		p.metrics.RecordNotificationSent(ctx, dr.LatencyMS)
		p.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
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
		p.writeAttempt(ctx, notifID, currentAttempt, &retryAfter, evt.Priority)

	default:
		p.metrics.RecordNotificationFailed(ctx, dr.LatencyMS)
		p.writeAttempt(ctx, notifID, currentAttempt, nil, evt.Priority)
		p.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
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

func (p *NotificationDeliveryPipeline) publishStatus(ctx context.Context, evt stream.NotificationDeliveryResultEvent) {
	if err := p.pub.Publish(ctx, stream.TopicStatus, evt); err != nil {
		slog.ErrorContext(ctx, "publish status failed", "id", evt.NotificationID, "error", err)
	}
}

func (p *NotificationDeliveryPipeline) writeAttempt(ctx context.Context, notifID uuid.UUID, attemptNumber int, retryAfter *time.Time, priority string) {
	if p.attempts == nil {
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
	if err := p.attempts.Create(ctx, a); err != nil {
		slog.ErrorContext(ctx, "write delivery attempt failed", "id", notifID, "error", err)
	}
}
