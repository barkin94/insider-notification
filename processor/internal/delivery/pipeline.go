package delivery

import (
	"context"
	"log/slog"
	"time"

	"github.com/barkin/insider-notification/processor/internal/service"
	"github.com/barkin/insider-notification/shared/lock"
	apipub "github.com/barkin/insider-notification/api/public"
	processorpub "github.com/barkin/insider-notification/processor/public"
	stream "github.com/barkin/insider-notification/shared/messaging"
)

// NotificationDeliveryPipeline runs a single notification event through
// each gate in sequence: locking, rate limiting, delivery, and outcome recording.
type NotificationDeliveryPipeline struct {
	pub            stream.Publisher
	deliveryClient service.NtfnDeliveryClient
	limiter        service.Limiter
	locker         lock.Locker
	metrics        service.Metrics
}

func NewNotificationDeliveryPipeline(
	pub stream.Publisher,
	deliveryClient service.NtfnDeliveryClient,
	limiter service.Limiter,
	locker lock.Locker,
	m service.Metrics,
) *NotificationDeliveryPipeline {
	return &NotificationDeliveryPipeline{
		pub:            pub,
		deliveryClient: deliveryClient,
		limiter:        limiter,
		locker:         locker,
		metrics:        m,
	}
}

// Run runs the notification through each gate in sequence.
// Gates own their logging; Run owns Ack/Nack and returns any infrastructure error so the caller can observe failures.
func (p *NotificationDeliveryPipeline) Run(ctx context.Context, result stream.Result[apipub.NotificationReadyEvent]) error {
	evt, msg := result.Event, result.Msg

	lockAcquired, err := p.locker.TryLock(ctx, evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "lock error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return err
	}

	// If lock is not acquired, it means another worker is processing the same notification, likely a retry.
	if !lockAcquired {
		slog.InfoContext(ctx, "lock miss, skipping", "id", evt.NotificationID)
		msg.Ack()
		return nil
	}
	defer p.locker.Unlock(ctx, evt.NotificationID) //nolint:errcheck

	limited, err := p.applyRateLimit(ctx, evt)
	if err != nil {
		msg.Nack()
		return err
	}
	if limited {
		msg.Ack()
		return nil
	}

	currentAttempt := evt.AttemptNumber + 1

	dr := p.deliveryClient.Send(ctx, evt.Recipient, evt.Channel, evt.Content)

	if err := p.handleDeliveryResult(ctx, evt, dr, currentAttempt); err != nil {
		msg.Nack()
		return err
	}
	msg.Ack()
	return nil
}

func (p *NotificationDeliveryPipeline) publishRetry(ctx context.Context, evt apipub.NotificationReadyEvent, attemptNumber int, scheduledAt time.Time) error {
	retryEvt := processorpub.NotificationRetryScheduleEvent{
		NotificationID: evt.NotificationID,
		Channel:        evt.Channel,
		Recipient:      evt.Recipient,
		Content:        evt.Content,
		Priority:       evt.Priority,
		MaxAttempts:    maxAttemptsFor(evt),
		AttemptNumber:  attemptNumber,
		ScheduledAt:    scheduledAt,
	}
	if err := p.pub.Publish(ctx, processorpub.TopicRetry, retryEvt); err != nil {
		slog.ErrorContext(ctx, "publish retry failed", "id", evt.NotificationID, "error", err)
		return err
	}
	return nil
}

// applyRateLimit checks the channel's token bucket. If exhausted, it defers
// the event via the ZSET and returns limited=true so the caller can Ack.
func (p *NotificationDeliveryPipeline) applyRateLimit(ctx context.Context, evt apipub.NotificationReadyEvent) (limited bool, err error) {
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
	retryAt := time.Now().Add(retryAfter).UTC()
	if err := p.publishRetry(ctx, evt, evt.AttemptNumber, retryAt); err != nil {
		return false, err
	}
	return true, nil
}

// recordOutcome schedules retryable delivery failures and publishes terminal
// status events for the API. It returns an error only when retry state could not
// be persisted, because the original stream message must remain unacked then.
func (p *NotificationDeliveryPipeline) handleDeliveryResult(ctx context.Context, evt apipub.NotificationReadyEvent, dr service.DeliveryResult, currentAttempt int) error {
	maxAttempts := maxAttemptsFor(evt)
	switch {
	case dr.Success:
		p.metrics.RecordNotificationSent(ctx, dr.LatencyMS)
		if err := p.publishStatus(ctx, processorpub.NotificationDeliveryResultEvent{
			NotificationID:    evt.NotificationID,
			Status:            string(apipub.StatusDelivered),
			AttemptNumber:     currentAttempt,
			HTTPStatusCode:    dr.StatusCode,
			ProviderMessageID: dr.ProviderMsgID,
			LatencyMS:         int(dr.LatencyMS),
		}); err != nil {
			return err
		}

	case dr.Retryable && currentAttempt < maxAttempts:
		scheduledAt := time.Now().Add(service.RetryDelay(currentAttempt + 1)).UTC()
		if err := p.publishRetry(ctx, evt, currentAttempt, scheduledAt); err != nil {
			return err
		}

	default:
		p.metrics.RecordNotificationFailed(ctx, dr.LatencyMS)
		if err := p.publishStatus(ctx, processorpub.NotificationDeliveryResultEvent{
			NotificationID: evt.NotificationID,
			Status:         string(apipub.StatusFailed),
			AttemptNumber:  currentAttempt,
			HTTPStatusCode: dr.StatusCode,
			ErrorMessage:   dr.ErrorMessage,
			LatencyMS:      int(dr.LatencyMS),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *NotificationDeliveryPipeline) publishStatus(ctx context.Context, evt processorpub.NotificationDeliveryResultEvent) error {
	if err := p.pub.Publish(ctx, processorpub.TopicStatus, evt); err != nil {
		slog.ErrorContext(ctx, "publish status failed", "id", evt.NotificationID, "error", err)
		return err
	}
	return nil
}

func maxAttemptsFor(evt apipub.NotificationReadyEvent) int {
	if evt.MaxAttempts > 0 {
		return evt.MaxAttempts
	}
	return service.MaxAttempts
}
