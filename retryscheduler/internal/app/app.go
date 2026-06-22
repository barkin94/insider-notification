package app

import (
	"context"
	"log/slog"
	"sync"

	processorpub "github.com/barkin94/insider-notification/processor/public"
	"github.com/barkin94/insider-notification/retryscheduler/internal/config"
	dbpostgres "github.com/barkin94/insider-notification/retryscheduler/internal/db/postgres"
	dispatcher "github.com/barkin94/insider-notification/retryscheduler/internal/notification_ready_dispatcher"
	"github.com/barkin94/insider-notification/retryscheduler/internal/transport/messaging"
	sharedbun "github.com/barkin94/insider-notification/shared/bun"
	stream "github.com/barkin94/insider-notification/shared/messaging"
	sharedredis "github.com/barkin94/insider-notification/shared/redis"
)

// App wires and runs the retryscheduler service.
type App struct {
	retryConsumer *messaging.RetryConsumer
	dispatcher    *dispatcher.NotificationReadyDispatcher
	wg            sync.WaitGroup
}

// New constructs all dependencies and returns a ready-to-run App.
// The returned cleanup func closes infrastructure and must be deferred by the caller.
func New(ctx context.Context, cfg *config.Config) (*App, func(), error) {
	rdb := sharedredis.NewClient(ctx, cfg.RedisAddr)
	bunDB := sharedbun.Connect(cfg.DatabaseURL)
	repo := dbpostgres.NewDeliveryAttemptRepository(bunDB)

	pub := stream.NewRedisPublisher(rdb)
	sub := stream.NewRedisSubscriber(rdb, "notify:cg:retryscheduler")
	msgs := stream.Subscribe[processorpub.NotificationRetryScheduleEvent](ctx, sub, processorpub.TopicRetry, cfg.OTelServiceName)

	cleanup := func() {
		_ = sub.Close()
		_ = rdb.Close()
		_ = bunDB.Close()
	}

	return &App{
		retryConsumer: messaging.NewRetryConsumer(repo, msgs),
		dispatcher:    dispatcher.NewNotificationReadyDispatcher(repo, pub, cfg.RetryDispatchInterval, cfg.RetryDispatchBatchSize),
	}, cleanup, nil
}

// Start launches the retry consumer and dispatcher in the background and returns
// a stop function the caller must invoke to wait for all goroutines to finish.
func (a *App) Start(ctx context.Context) func() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.retryConsumer.Run(ctx)
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.dispatcher.Run(ctx)
	}()

	slog.Info("retryscheduler started")
	return a.wg.Wait
}
