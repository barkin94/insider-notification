package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/barkin/insider-notification/api/internal/domain/notification"
	"github.com/barkin/insider-notification/api/internal/repository"
	"github.com/barkin/insider-notification/shared/stream"
)

var topicByPriority = map[string]string{
	string(notification.PriorityHigh):   stream.TopicHigh,
	string(notification.PriorityNormal): stream.TopicNormal,
	string(notification.PriorityLow):    stream.TopicLow,
}

// NotificationScheduleReader is the narrow read port for scheduled notifications.
type NotificationScheduleReader interface {
	FindScheduledDue(ctx context.Context) ([]*repository.Notification, error)
}

// Scheduler polls for scheduled notifications that are due and publishes them.
type Scheduler struct {
	repo      NotificationScheduleReader
	publisher stream.Publisher
	interval  time.Duration
}

func New(repo NotificationScheduleReader, publisher stream.Publisher, interval time.Duration) *Scheduler {
	return &Scheduler{
		repo:      repo,
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
	s.dispatchScheduled(ctx)
}

func (s *Scheduler) dispatchScheduled(ctx context.Context) {
	notifications, err := s.repo.FindScheduledDue(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler: find scheduled due", "error", err)
		return
	}
	for _, n := range notifications {
		evt := stream.NotificationReadyEvent{
			NotificationID: n.ID.String(),
			Channel:        n.Channel,
			Recipient:      n.Recipient,
			Content:        n.Content,
			Priority:       n.Priority,
			MaxAttempts:    n.MaxAttempts,
		}
		topic := topicByPriority[n.Priority]
		if err := s.publisher.Publish(ctx, topic, evt); err != nil {
			slog.ErrorContext(ctx, "scheduler: publish scheduled", "id", n.ID, "error", err)
		}
	}
}
