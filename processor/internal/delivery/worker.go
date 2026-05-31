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

// NotificationDeliveryPipelineWorker runs a single notification event through
// each gate in sequence: locking, rate limiting, delivery, and outcome recording.
type NotificationDeliveryPipelineWorker struct {
	pub            stream.Publisher
	deliveryClient service.NtfnDeliveryClient
	limiter        service.Limiter
	locker         lock.Locker
	attempts       processordb.DeliveryAttemptRepository
	metrics        service.Metrics
}

func NewNotificationDeliveryPipelineWorker(
	pub stream.Publisher,
	deliveryClient service.NtfnDeliveryClient,
	limiter service.Limiter,
	locker lock.Locker,
	attempts processordb.DeliveryAttemptRepository,
	m service.Metrics,
) *NotificationDeliveryPipelineWorker {
	return &NotificationDeliveryPipelineWorker{
		pub:            pub,
		deliveryClient: deliveryClient,
		limiter:        limiter,
		locker:         locker,
		attempts:       attempts,
		metrics:        m,
	}
}

// Run runs the notification through each gate in sequence.
// Gates own their logging; Run owns Ack/Nack.
func (p *NotificationDeliveryPipelineWorker) Run(ctx context.Context, result stream.Result[stream.NotificationReadyEvent]) {
	ctx, span := otel.Tracer("processor").Start(ctx, "deliver")
	defer span.End()

	evt, msg := result.Event, result.Msg

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
func (p *NotificationDeliveryPipelineWorker) countPriorAttempts(ctx context.Context, id uuid.UUID) int {
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

// applyRateLimit checks the channel's token bucket. If exhausted, it re-enqueues
// the event and returns limited=true so the caller can Ack and move on.
func (p *NotificationDeliveryPipelineWorker) applyRateLimit(ctx context.Context, evt stream.NotificationReadyEvent) (limited bool, err error) {
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
func (p *NotificationDeliveryPipelineWorker) recordOutcome(ctx context.Context, evt stream.NotificationReadyEvent, dr service.DeliveryResult, notifID uuid.UUID, currentAttempt int) {
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
		p.writeAttempt(ctx, evt, notifID, currentAttempt, &retryAfter)

	default:
		p.metrics.RecordNotificationFailed(ctx, dr.LatencyMS)
		p.writeAttempt(ctx, evt, notifID, currentAttempt, nil)
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

func (p *NotificationDeliveryPipelineWorker) publishStatus(ctx context.Context, evt stream.NotificationDeliveryResultEvent) {
	if err := p.pub.Publish(ctx, stream.TopicStatus, evt); err != nil {
		slog.ErrorContext(ctx, "publish status failed", "id", evt.NotificationID, "error", err)
	}
}

func (p *NotificationDeliveryPipelineWorker) writeAttempt(ctx context.Context, evt stream.NotificationReadyEvent, notifID uuid.UUID, attemptNumber int, retryAfter *time.Time) {
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
		Priority:       evt.Priority,
		RetryAfter:     retryAfter,
		Channel:        evt.Channel,
		Recipient:      evt.Recipient,
		Content:        evt.Content,
		MaxAttempts:    evt.MaxAttempts,
		Metadata:       evt.Metadata,
	}
	a.ID = id
	if err := p.attempts.Create(ctx, a); err != nil {
		slog.ErrorContext(ctx, "write delivery attempt failed", "id", notifID, "error", err)
	}
}
