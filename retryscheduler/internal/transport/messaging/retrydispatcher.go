package messaging

import (
	"context"
	"log/slog"
	"time"

	schedulerdb "github.com/barkin/insider-notification/retryscheduler/internal/db"
	"github.com/barkin/insider-notification/shared/stream"
)

// RetryDispatcher republishes due retry attempts without occupying delivery workers
// during backoff waits.
type RetryDispatcher struct {
	repo     schedulerdb.DeliveryAttemptRepository
	pub      stream.Publisher
	interval time.Duration
	batch    int
}

func NewRetryDispatcher(repo schedulerdb.DeliveryAttemptRepository, pub stream.Publisher, interval time.Duration, batch int) *RetryDispatcher {
	if interval <= 0 {
		interval = time.Second
	}
	if batch < 1 {
		batch = 100
	}
	return &RetryDispatcher{
		repo:     repo,
		pub:      pub,
		interval: interval,
		batch:    batch,
	}
}

func (d *RetryDispatcher) Run(ctx context.Context) {
	if d == nil {
		return
	}
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.Tick(ctx)
		}
	}
}

func (d *RetryDispatcher) Tick(ctx context.Context) {
	attempts, err := d.repo.FindDueBefore(ctx, time.Now().UTC(), d.batch)
	if err != nil {
		slog.ErrorContext(ctx, "retry dispatcher: read due attempts", "error", err)
		return
	}
	for _, a := range attempts {
		evt := stream.NotificationReadyEvent{
			NotificationID: a.NotificationID,
			Channel:        a.Channel,
			Recipient:      a.Recipient,
			Content:        a.Content,
			Priority:       a.Priority,
			MaxAttempts:    a.MaxAttempts,
			AttemptNumber:  a.AttemptNumber,
		}
		topic := topicForPriority(a.Priority)
		if err := d.pub.Publish(ctx, topic, evt); err != nil {
			slog.ErrorContext(ctx, "retry dispatcher: publish retry", "id", a.NotificationID, "error", err)
			continue
		}
		if err := d.repo.DeleteByID(ctx, a.NotificationID); err != nil {
			slog.ErrorContext(ctx, "retry dispatcher: delete attempt", "id", a.NotificationID, "error", err)
		}
	}
}
