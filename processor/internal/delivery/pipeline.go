package delivery

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apipub "github.com/barkin94/insider-notification/api/public"
	"github.com/barkin94/insider-notification/processor/internal/service"
	processorpub "github.com/barkin94/insider-notification/processor/public"
	"github.com/barkin94/insider-notification/shared/lock"
	stream "github.com/barkin94/insider-notification/shared/messaging"
)

// ErrRetryAfter is returned by Run when the message should be redelivered via
// NakWithDelay after the given duration. It is not a fatal error.
type ErrRetryAfter struct {
	Delay time.Duration
}

func (e ErrRetryAfter) Error() string {
	return fmt.Sprintf("retry after %s", e.Delay)
}

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

// Run processes one notification event. attemptNumber is the 1-indexed NATS delivery
// count for this message (1 = first attempt, 2 = first retry, etc.).
// Returns ErrRetryAfter when the caller should NakWithDelay, nil on success or
// terminal failure (caller should Ack), or another error on infrastructure failure
// (caller should Nack).
func (p *NotificationDeliveryPipeline) Run(ctx context.Context, evt apipub.NotificationReadyEvent, attemptNumber int) error {
	lockAcquired, err := p.locker.TryLock(ctx, evt.NotificationID)
	if err != nil {
		slog.ErrorContext(ctx, "lock error", "id", evt.NotificationID, "error", err)
		return err
	}

	// If lock is not acquired, another worker is already processing this notification.
	if !lockAcquired {
		slog.InfoContext(ctx, "lock miss, skipping", "id", evt.NotificationID)
		return nil
	}
	defer p.locker.Unlock(ctx, evt.NotificationID) //nolint:errcheck

	if err := p.applyRateLimit(ctx, evt); err != nil {
		return err // ErrRetryAfter or infra error — both propagate to caller
	}

	dr := p.deliveryClient.Send(ctx, evt.Recipient, evt.Channel, evt.Content)

	return p.handleDeliveryResult(ctx, evt, dr, attemptNumber)
}

// applyRateLimit checks the channel's token bucket. Returns ErrRetryAfter when
// the bucket is exhausted so the caller can NakWithDelay without counting the
// attempt against MaxAttempts. Returns nil when the request is allowed through.
func (p *NotificationDeliveryPipeline) applyRateLimit(ctx context.Context, evt apipub.NotificationReadyEvent) error {
	allowed, retryAfter, err := p.limiter.IsAllowed(ctx, evt.Channel)
	if err != nil {
		slog.ErrorContext(ctx, "rate limit error", "id", evt.NotificationID, "error", err)
		return err
	}
	if allowed {
		return nil
	}
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	slog.InfoContext(ctx, "rate limited, naking with delay", "id", evt.NotificationID, "channel", evt.Channel, "delay", retryAfter)
	return ErrRetryAfter{Delay: retryAfter}
}

// handleDeliveryResult returns ErrRetryAfter for retryable failures with remaining
// attempts, nil after publishing a terminal status event, or an error if the status
// event could not be published.
func (p *NotificationDeliveryPipeline) handleDeliveryResult(ctx context.Context, evt apipub.NotificationReadyEvent, dr service.DeliveryResult, attemptNumber int) error {
	currentAttempt := attemptNumber
	maxAttempts := maxAttemptsFor(evt)
	switch {
	case dr.Success:
		p.metrics.RecordNotificationSent(ctx, dr.LatencyMS)
		return p.publishStatus(ctx, processorpub.NotificationDeliveryResultEvent{
			NotificationID:    evt.NotificationID,
			Status:            string(apipub.StatusDelivered),
			AttemptNumber:     currentAttempt,
			HTTPStatusCode:    dr.StatusCode,
			ProviderMessageID: dr.ProviderMsgID,
			LatencyMS:         int(dr.LatencyMS),
		})

	case dr.Retryable && currentAttempt < maxAttempts:
		delay := service.RetryDelay(currentAttempt + 1)
		slog.InfoContext(ctx, "retryable failure, naking with delay", "id", evt.NotificationID, "attempt", currentAttempt, "delay", delay)
		return ErrRetryAfter{Delay: delay}

	default:
		p.metrics.RecordNotificationFailed(ctx, dr.LatencyMS)
		return p.publishStatus(ctx, processorpub.NotificationDeliveryResultEvent{
			NotificationID: evt.NotificationID,
			Status:         string(apipub.StatusFailed),
			AttemptNumber:  currentAttempt,
			HTTPStatusCode: dr.StatusCode,
			ErrorMessage:   dr.ErrorMessage,
			LatencyMS:      int(dr.LatencyMS),
		})
	}
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
