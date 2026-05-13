package scheduler

import (
	"context"
	"log/slog"
	"time"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// NotificationRow holds the fields the scheduler needs from public.notifications.
type NotificationRow struct {
	ID           uuid.UUID `bun:"id"`
	Priority     string    `bun:"priority"`
	Channel      string    `bun:"channel"`
	Recipient    string    `bun:"recipient"`
	Content      string    `bun:"content"`
	MaxAttempts  int       `bun:"max_attempts"`
}

// NotificationReader reads notifications from public.notifications.
type NotificationReader interface {
	FindScheduledDue(ctx context.Context) ([]NotificationRow, error)
	FindByIDs(ctx context.Context, ids []uuid.UUID) ([]NotificationRow, error)
}

// RetryReader reads failed delivery attempts that are due for retry.
type RetryReader interface {
	FindDueRetries(ctx context.Context) ([]*processordb.DeliveryAttempt, error)
}

// StreamPublisher publishes events to a stream topic.
type StreamPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

var topicByPriority = map[string]string{
	model.PriorityHigh:   stream.TopicHigh,
	model.PriorityNormal: stream.TopicNormal,
	model.PriorityLow:    stream.TopicLow,
}

// Scheduler polls for due notifications and retries and publishes them.
type Scheduler struct {
	notifs    NotificationReader
	retries   RetryReader
	publisher StreamPublisher
	interval  time.Duration
}

func New(notifs NotificationReader, retries RetryReader, publisher StreamPublisher) *Scheduler {
	return &Scheduler{
		notifs:    notifs,
		retries:   retries,
		publisher: publisher,
		interval:  5 * time.Second,
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
	s.dispatchRetries(ctx)
}

func (s *Scheduler) dispatchScheduled(ctx context.Context) {
	rows, err := s.notifs.FindScheduledDue(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler: find scheduled due", "error", err)
		return
	}
	for _, n := range rows {
		evt := stream.NotificationCreatedEvent{
			NotificationID: n.ID.String(),
			Channel:        n.Channel,
			Recipient:      n.Recipient,
			Content:        n.Content,
			Priority:       n.Priority,
			AttemptNumber:  1,
			MaxAttempts:    n.MaxAttempts,
		}
		topic := topicByPriority[n.Priority]
		if err := s.publisher.Publish(ctx, topic, evt); err != nil {
			slog.ErrorContext(ctx, "scheduler: publish scheduled", "id", n.ID, "error", err)
		}
	}
}

func (s *Scheduler) dispatchRetries(ctx context.Context) {
	attempts, err := s.retries.FindDueRetries(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler: find due retries", "error", err)
		return
	}
	if len(attempts) == 0 {
		return
	}

	ids := make([]uuid.UUID, len(attempts))
	for i, a := range attempts {
		ids[i] = a.NotificationID
	}
	notifs, err := s.notifs.FindByIDs(ctx, ids)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler: fetch notifications for retries", "error", err)
		return
	}
	notifByID := make(map[uuid.UUID]NotificationRow, len(notifs))
	for _, n := range notifs {
		notifByID[n.ID] = n
	}

	for _, a := range attempts {
		n, ok := notifByID[a.NotificationID]
		if !ok {
			slog.ErrorContext(ctx, "scheduler: notification not found for retry", "id", a.NotificationID)
			continue
		}
		evt := stream.NotificationCreatedEvent{
			NotificationID: a.NotificationID.String(),
			Channel:        n.Channel,
			Recipient:      n.Recipient,
			Content:        n.Content,
			Priority:       a.Priority,
			AttemptNumber:  a.AttemptNumber + 1,
			MaxAttempts:    n.MaxAttempts,
		}
		topic := topicByPriority[a.Priority]
		if err := s.publisher.Publish(ctx, topic, evt); err != nil {
			slog.ErrorContext(ctx, "scheduler: publish retry", "id", a.NotificationID, "error", err)
		}
	}
}

// bunNotificationReader implements NotificationReader against public.notifications.
type bunNotificationReader struct{ db *bun.DB }

func NewNotificationReader(db *bun.DB) NotificationReader {
	return &bunNotificationReader{db: db}
}

func (r *bunNotificationReader) FindScheduledDue(ctx context.Context) ([]NotificationRow, error) {
	var rows []NotificationRow
	err := r.db.NewSelect().
		TableExpr("notifications").
		ColumnExpr("id, priority, channel, recipient, content, max_attempts").
		Where("deliver_after IS NOT NULL").
		Where("deliver_after <= NOW()").
		Where("status = ?", model.StatusPending).
		OrderExpr("deliver_after ASC").
		Limit(500).
		Scan(ctx, &rows)
	return rows, err
}

func (r *bunNotificationReader) FindByIDs(ctx context.Context, ids []uuid.UUID) ([]NotificationRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []NotificationRow
	err := r.db.NewSelect().
		TableExpr("notifications").
		ColumnExpr("id, priority, channel, recipient, content, max_attempts").
		Where("id IN (?)", bun.List(ids)).
		Scan(ctx, &rows)
	return rows, err
}
