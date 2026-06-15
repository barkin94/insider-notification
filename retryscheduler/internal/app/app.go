package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/barkin/insider-notification/retryscheduler/internal/config"
	schedulerdb "github.com/barkin/insider-notification/retryscheduler/internal/db"
	"github.com/barkin/insider-notification/retryscheduler/internal/transport/messaging"
	shareddb "github.com/barkin/insider-notification/shared/db"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
)

// App wires and runs the retryscheduler service.
type App struct {
	retryConsumer   *messaging.RetryConsumer
	retryDispatcher *messaging.RetryDispatcher
}

// New constructs all dependencies and returns a ready-to-run App.
// The returned cleanup func closes infrastructure and must be deferred by the caller.
func New(ctx context.Context, cfg *config.Config) (*App, func(), error) {
	rdb := sharedredis.NewClient(ctx, cfg.RedisAddr)
	bunDB := shareddb.Open(cfg.DatabaseURL)
	repo := schedulerdb.NewDeliveryAttemptRepository(bunDB)

	pub := stream.NewRedisPublisher(rdb)
	sub := stream.NewRedisSubscriber(rdb, "notify:cg:retryscheduler")
	msgs := stream.Subscribe[stream.NotificationRetryScheduleEvent](ctx, sub, stream.TopicRetry, cfg.OTelServiceName)

	cleanup := func() {
		_ = sub.Close()
		_ = rdb.Close()
		_ = bunDB.Close()
	}

	return &App{
		retryConsumer:   messaging.NewRetryConsumer(repo, msgs),
		retryDispatcher: messaging.NewRetryDispatcher(repo, pub, cfg.RetryDispatchInterval, cfg.RetryDispatchBatchSize),
	}, cleanup, nil
}

// Run starts the retry consumer and dispatcher, blocks until ctx is cancelled,
// then waits for all goroutines to finish.
func (a *App) Run(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.retryConsumer.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.retryDispatcher.Run(ctx)
	}()

	slog.Info("retryscheduler started")

	<-ctx.Done()
	slog.Info("shutting down, waiting for goroutines")
	wg.Wait()
	slog.Info("all goroutines stopped")
}
