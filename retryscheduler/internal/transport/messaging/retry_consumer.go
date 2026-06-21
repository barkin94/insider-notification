package messaging

import (
	"context"
	"log/slog"

	schedulerdb "github.com/barkin/insider-notification/retryscheduler/internal/db"
	processorpub "github.com/barkin/insider-notification/processor/public"
	stream "github.com/barkin/insider-notification/shared/messaging"
)

// RetryConsumer consumes NotificationRetryScheduleEvents from TopicRetry and
// persists each one to Postgres so the RetryDispatcher picks it up when ScheduledAt is past.
type RetryConsumer struct {
	repo schedulerdb.DeliveryAttemptRepository
	msgs <-chan stream.Result[processorpub.NotificationRetryScheduleEvent]
}

func NewRetryConsumer(repo schedulerdb.DeliveryAttemptRepository, msgs <-chan stream.Result[processorpub.NotificationRetryScheduleEvent]) *RetryConsumer {
	return &RetryConsumer{repo: repo, msgs: msgs}
}

func (c *RetryConsumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-c.msgs:
			if !ok {
				return
			}
			c.handleRetryEvent(result.Ctx, result)
		}
	}
}

func (c *RetryConsumer) handleRetryEvent(ctx context.Context, result stream.Result[processorpub.NotificationRetryScheduleEvent]) {
	evt := result.Event
	msg := result.Msg

	scheduledAt := evt.ScheduledAt
	attempt := &schedulerdb.DeliveryAttempt{
		NotificationID: evt.NotificationID,
		Channel:        evt.Channel,
		Recipient:      evt.Recipient,
		Content:        evt.Content,
		Priority:       evt.Priority,
		MaxAttempts:    evt.MaxAttempts,
		AttemptNumber:  evt.AttemptNumber,
		RetryAfter:     &scheduledAt,
	}
	if err := c.repo.Upsert(ctx, attempt); err != nil {
		slog.ErrorContext(ctx, "persist retry schedule failed", "id", evt.NotificationID, "error", err)
		msg.Nack()
		return
	}
	slog.InfoContext(ctx, "retry scheduled", "id", evt.NotificationID, "attempt", evt.AttemptNumber, "scheduled_at", scheduledAt)
	msg.Ack()
}
