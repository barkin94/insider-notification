package messaging

import (
	"context"
	"log/slog"
	"sync"
	"time"

	apipub "github.com/barkin94/insider-notification/api/public"
	schedulerdb "github.com/barkin94/insider-notification/retryscheduler/internal/db"
	stream "github.com/barkin94/insider-notification/shared/messaging"
	sharedotel "github.com/barkin94/insider-notification/shared/otel"
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
	attempts, err := d.repo.DeleteByRetryAfterBeforeReturning(ctx, time.Now().UTC(), d.batch)
	if err != nil {
		slog.ErrorContext(ctx, "retry dispatcher: claim due attempts", "error", err)
		return
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed []*schedulerdb.DeliveryAttempt
	)
	for _, a := range attempts {
		wg.Add(1)
		go func(a *schedulerdb.DeliveryAttempt) {
			defer wg.Done()
			attemptCtx := sharedotel.ContextWithTraceMetadata(ctx, a.TraceMetadata)
			evt := apipub.NotificationReadyEvent{
				NotificationID: a.NotificationID,
				Channel:        a.Channel,
				Recipient:      a.Recipient,
				Content:        a.Content,
				Priority:       a.Priority,
				MaxAttempts:    a.MaxAttempts,
				AttemptNumber:  a.AttemptNumber,
			}
			topic := apipub.TopicByPriority[apipub.Priority(a.Priority)]
			if err := d.pub.Publish(attemptCtx, string(topic), evt); err != nil {
				slog.ErrorContext(attemptCtx, "retry dispatcher: publish retry", "id", a.NotificationID, "error", err)
				mu.Lock()
				failed = append(failed, a)
				mu.Unlock()
			}
		}(a)
	}
	wg.Wait()

	if len(failed) > 0 {
		if err := d.repo.UpsertAll(ctx, failed); err != nil {
			slog.ErrorContext(ctx, "retry dispatcher: re-enqueue after publish failures", "count", len(failed), "error", err)
		}
	}
}
