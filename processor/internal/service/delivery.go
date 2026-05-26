package service

import (
	"context"
	"log/slog"
	"time"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

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

// DeliveryService processes notification delivery events.
type DeliveryService struct {
	pub            stream.Publisher
	deliveryClient DeliveryClient
	limiter        Limiter
	locker         lock.Locker
	cancel         CancellationStore
	attempts       DeliveryAttemptWriter
}

func NewDeliveryService(
	pub stream.Publisher,
	deliveryClient DeliveryClient,
	limiter Limiter,
	locker lock.Locker,
	cancel CancellationStore,
	attempts DeliveryAttemptWriter,
) *DeliveryService {
	return &DeliveryService{
		pub:            pub,
		deliveryClient: deliveryClient,
		limiter:        limiter,
		locker:         locker,
		cancel:         cancel,
		attempts:       attempts,
	}
}

// Process handles a single notification delivery event.
// It calls result.Msg.Ack() or result.Msg.Nack() before returning.
func (s *DeliveryService) Process(ctx context.Context, result stream.Result[stream.NotificationCreatedEvent]) {
	ctx, span := otel.Tracer("processor").Start(ctx, "Process")
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
	cancelled, err := s.cancel.IsCancelled(ctx, evt.NotificationID)
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
	locked, err := s.locker.TryLock(ctx, evt.NotificationID)
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
	defer s.locker.Unlock(ctx, evt.NotificationID) //nolint:errcheck

	// rate limit
	allowed, err := s.limiter.Allow(ctx, evt.Channel)
	if err != nil {
		slog.ErrorContext(ctx, "rate limit error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}
	if !allowed {
		slog.InfoContext(ctx, "rate limited, re-enqueuing", "id", evt.NotificationID, "channel", evt.Channel)
		if err := s.pub.Publish(ctx, topicByPriority[evt.Priority], evt); err != nil {
			slog.ErrorContext(ctx, "re-enqueue rate-limited failed", "id", evt.NotificationID, "error", err)
			msg.Nack()
			return
		}
		msg.Ack()
		return
	}

	dr, err := s.deliveryClient.Send(ctx, evt.Recipient, evt.Channel, evt.Content)
	if err != nil {
		slog.ErrorContext(ctx, "delivery transport error", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case dr.Success:
		s.writeAttempt(ctx, evt.NotificationID, evt.AttemptNumber, model.StatusDelivered, nil, evt.Priority)
		s.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
			NotificationID:    evt.NotificationID,
			Status:            model.StatusDelivered,
			AttemptNumber:     evt.AttemptNumber,
			HTTPStatusCode:    dr.StatusCode,
			ProviderMessageID: dr.ProviderMsgID,
			LatencyMS:         int(dr.LatencyMS),
			UpdatedAt:         now,
		})

	case dr.Retryable && evt.AttemptNumber < evt.MaxAttempts:
		retryAfter := time.Now().Add(RetryDelay(evt.AttemptNumber + 1)).UTC()
		s.writeAttempt(ctx, evt.NotificationID, evt.AttemptNumber, model.StatusFailed, &retryAfter, evt.Priority)
	default:
		s.writeAttempt(ctx, evt.NotificationID, evt.AttemptNumber, model.StatusFailed, nil, evt.Priority)
		s.publishStatus(ctx, stream.NotificationDeliveryResultEvent{
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

func (s *DeliveryService) publishStatus(ctx context.Context, evt stream.NotificationDeliveryResultEvent) {
	if err := s.pub.Publish(ctx, stream.TopicStatus, evt); err != nil {
		slog.ErrorContext(ctx, "publish status failed", "id", evt.NotificationID, "error", err)
	}
}

func (s *DeliveryService) writeAttempt(ctx context.Context, notifIDStr string, attemptNumber int, status string, retryAfter *time.Time, priority string) {
	if s.attempts == nil {
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
	if err := s.attempts.Create(ctx, a); err != nil {
		slog.ErrorContext(ctx, "write delivery attempt failed", "id", notifIDStr, "error", err)
	}
}
