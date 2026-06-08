package delivery

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/service"
	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
)

// NotificationDeliveryPipeline runs a single notification event through
// each gate in sequence: locking, rate limiting, delivery, and outcome recording.
type NotificationDeliveryPipeline struct {
	pub            stream.Publisher
	deliveryClient service.NtfnDeliveryClient
	limiter        service.Limiter
	locker         lock.Locker
	attempts       processordb.DeliveryAttemptRepository
	metrics        service.Metrics
}

func NewNotificationDeliveryPipeline(
	pub stream.Publisher,
	deliveryClient service.NtfnDeliveryClient,
	limiter service.Limiter,
	locker lock.Locker,
	attempts processordb.DeliveryAttemptRepository,
	m service.Metrics,
) *NotificationDeliveryPipeline {
	return &NotificationDeliveryPipeline{
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
func (p *NotificationDeliveryPipeline) Run(ctx context.Context, result stream.Result[stream.NotificationReadyEvent]) {
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

	currentAttempt := p.getLastAttemptNumber(ctx, evt.NotificationID) + 1

	dr := p.deliveryClient.Send(ctx, evt.Recipient, evt.Channel, evt.Content)

	if err := p.recordOutcome(ctx, evt, dr, currentAttempt); err != nil {
		msg.Nack()
		return
	}
	msg.Ack()
}

// savePayload persists the notification payload so the retry dispatcher can reconstruct the event.
// Must be called before any Delay or Create. Idempotent: subsequent calls for the same notification are no-ops.
func (p *NotificationDeliveryPipeline) savePayload(ctx context.Context, evt stream.NotificationReadyEvent) error {
	err := p.attempts.SavePayload(ctx, &processordb.DeliveryAttempt{
		NotificationID: evt.NotificationID,
		Channel:        evt.Channel,
		Recipient:      evt.Recipient,
		Content:        evt.Content,
		Priority:       evt.Priority,
		MaxAttempts:    maxAttemptsFor(evt),
	})

	if err != nil {
		slog.ErrorContext(ctx, "save payload failed", "id", evt.NotificationID, "error", err)
	}

	return err
}

// getLastAttemptNumber returns the last recorded attempt number from Redis.
// Returns 0 on error or when no attempt has been recorded yet (first delivery).
func (p *NotificationDeliveryPipeline) getLastAttemptNumber(ctx context.Context, id string) int {
	n, err := p.attempts.GetAttemptNumber(ctx, id)
	if err != nil {
		slog.ErrorContext(ctx, "get last attempt number failed", "id", id, "error", err)
		return 0
	}
	return n
}

// applyRateLimit checks the channel's token bucket. If exhausted, it defers
// the event via the ZSET and returns limited=true so the caller can Ack.
func (p *NotificationDeliveryPipeline) applyRateLimit(ctx context.Context, evt stream.NotificationReadyEvent) (limited bool, err error) {
	allowed, retryAfter, err := p.limiter.IsAllowed(ctx, evt.Channel)
	if err != nil {
		slog.ErrorContext(ctx, "rate limit error", "id", evt.NotificationID, "error", err)
		return false, err
	}
	if allowed {
		return false, nil
	}
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	slog.InfoContext(ctx, "rate limited, scheduling retry", "id", evt.NotificationID, "channel", evt.Channel, "retry_after", retryAfter)
	if err := p.savePayload(ctx, evt); err != nil {
		return false, err
	}
	if err := p.attempts.Delay(ctx, evt.NotificationID, time.Now().Add(retryAfter).UTC()); err != nil {
		slog.ErrorContext(ctx, "delay notification failed", "id", evt.NotificationID, "error", err)
		return false, err
	}
	return true, nil
}

// recordOutcome schedules retryable delivery failures and publishes terminal
// status events for the API. It returns an error only when retry state could not
// be persisted, because the original stream message must remain unacked then.
func (p *NotificationDeliveryPipeline) recordOutcome(ctx context.Context, evt stream.NotificationReadyEvent, dr service.DeliveryResult, currentAttempt int) error {
	notifID := evt.NotificationID
	maxAttempts := maxAttemptsFor(evt)
	switch {
	case dr.Success:
		p.metrics.RecordNotificationSent(ctx, dr.LatencyMS)
		if err := p.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
			NotificationID:    evt.NotificationID,
			Status:            string(model.StatusDelivered),
			AttemptNumber:     currentAttempt,
			HTTPStatusCode:    dr.StatusCode,
			ProviderMessageID: dr.ProviderMsgID,
			LatencyMS:         int(dr.LatencyMS),
		}); err != nil {
			return err
		}
		p.clearAttempt(ctx, notifID)

	case dr.Retryable && currentAttempt < maxAttempts:
		retryAfter := time.Now().Add(service.RetryDelay(currentAttempt + 1)).UTC()
		if err := p.savePayload(ctx, evt); err != nil {
			return err
		}
		if err := p.attempts.Create(ctx, &processordb.DeliveryAttempt{
			NotificationID: notifID,
			AttemptNumber:  currentAttempt,
			RetryAfter:     &retryAfter,
		}); err != nil {
			slog.ErrorContext(ctx, "schedule delivery retry failed", "id", notifID, "error", err)
			return err
		}

	default:
		p.metrics.RecordNotificationFailed(ctx, dr.LatencyMS)
		if err := p.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
			NotificationID: evt.NotificationID,
			Status:         string(model.StatusFailed),
			AttemptNumber:  currentAttempt,
			HTTPStatusCode: dr.StatusCode,
			ErrorMessage:   dr.ErrorMessage,
			LatencyMS:      int(dr.LatencyMS),
		}); err != nil {
			return err
		}
		p.clearAttempt(ctx, notifID)
	}
	return nil
}

func (p *NotificationDeliveryPipeline) publishStatus(ctx context.Context, evt stream.NotificationDeliveryResultEvent) error {
	if err := p.pub.Publish(ctx, stream.TopicStatus, evt); err != nil {
		slog.ErrorContext(ctx, "publish status failed", "id", evt.NotificationID, "error", err)
		return err
	}
	return nil
}

func (p *NotificationDeliveryPipeline) clearAttempt(ctx context.Context, notifID string) {
	if err := p.attempts.Delete(ctx, notifID); err != nil {
		slog.ErrorContext(ctx, "delete delivery attempt failed", "id", notifID, "error", err)
	}
}

func maxAttemptsFor(evt stream.NotificationReadyEvent) int {
	if evt.MaxAttempts > 0 {
		return evt.MaxAttempts
	}
	return service.MaxAttempts
}
