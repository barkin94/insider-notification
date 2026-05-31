package scheduler

import (
	"context"
	"log/slog"
	"time"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
)

var topicByPriority = map[string]string{
	model.PriorityHigh:   stream.TopicHigh,
	model.PriorityNormal: stream.TopicNormal,
	model.PriorityLow:    stream.TopicLow,
}

// Scheduler polls for due retries and re-publishes them.
type Scheduler struct {
	retries   processordb.DeliveryAttemptRepository
	publisher stream.Publisher
	interval  time.Duration
}

func New(retries processordb.DeliveryAttemptRepository, publisher stream.Publisher, interval time.Duration) *Scheduler {
	return &Scheduler{
		retries:   retries,
		publisher: publisher,
		interval:  interval,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

func (s *Scheduler) Tick(ctx context.Context) {
	s.dispatchRetries(ctx)
}

func (s *Scheduler) dispatchRetries(ctx context.Context) {
	attempts, err := s.retries.FindDueRetries(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler: find due retries", "error", err)
		return
	}
	for _, a := range attempts {
		evt := stream.NotificationReadyEvent{
			NotificationID: a.NotificationID.String(),
			Channel:        a.Channel,
			Recipient:      a.Recipient,
			Content:        a.Content,
			Priority:       a.Priority,
			MaxAttempts:    a.MaxAttempts,
			Metadata:       a.Metadata,
		}
		topic := topicByPriority[a.Priority]
		if err := s.publisher.Publish(ctx, topic, evt); err != nil {
			slog.ErrorContext(ctx, "scheduler: publish retry", "id", a.NotificationID, "error", err)
		}
	}
}
