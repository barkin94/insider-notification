package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-redis/redis_rate/v10"

	"github.com/barkin/insider-notification/processor/internal/config"
	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/service"
	"github.com/barkin/insider-notification/processor/internal/transport/messaging"
	shareddb "github.com/barkin/insider-notification/shared/db"
	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
)

// App wires and runs the processor service.
type App struct {
	workerPool      *delivery.NotificationDeliveryWorkerPool
	retryDispatcher *messaging.RetryDispatcher
}

// New constructs all dependencies and returns a ready-to-run App.
// The returned cleanup func closes infrastructure and must be deferred by the caller.
func New(ctx context.Context, cfg *config.Config) (*App, func(), error) {
	rdb := sharedredis.NewClient(ctx, cfg.RedisAddr)

	bunDB := shareddb.Open(cfg.DatabaseURL)
	attemptRepo := processordb.NewDeliveryAttemptRepository(bunDB)

	pub := stream.NewRedisPublisher(rdb)
	sub := stream.NewRedisSubscriber(rdb, "notify:cg:processor")

	m, err := service.NewMetrics(rdb)
	if err != nil {
		_ = sub.Close()
		return nil, nil, fmt.Errorf("init metrics: %w", err)
	}

	pipeline := delivery.NewNotificationDeliveryPipeline(
		pub,
		service.NewNtfnDeliveryClient(cfg.NtfnDeliveryClientURL, cfg.NtfnDeliveryClientTimeout),
		service.NewLimiter(rdb, map[string]redis_rate.Limit{
			string(model.ChannelSMS):   {Rate: cfg.SMSRatePerSecond, Burst: cfg.SMSBurst, Period: time.Second},
			string(model.ChannelEmail): {Rate: cfg.EmailRatePerSecond, Burst: cfg.EmailBurst, Period: time.Second},
			string(model.ChannelPush):  {Rate: cfg.PushRatePerSecond, Burst: cfg.PushBurst, Period: time.Second},
		}),
		lock.NewRedisLocker(rdb),
		attemptRepo,
		m,
	)
	retryDispatcher := messaging.NewRetryDispatcher(attemptRepo, pub, cfg.RetryDispatchInterval, cfg.RetryDispatchBatchSize)

	cleanup := func() {
		_ = sub.Close()
		_ = rdb.Close()
		_ = bunDB.Close()
	}

	router := messaging.NewNotificationRouter(ctx, sub, cfg.OTelServiceName, cfg.HighWeight, cfg.NormalWeight, cfg.LowWeight)

	return &App{
		workerPool:      delivery.NewNotificationDeliveryWorkerPool(router, pipeline, cfg.WorkerConcurrency),
		retryDispatcher: retryDispatcher,
	}, cleanup, nil
}

// Run starts the retry dispatcher and workerPool pool, blocks until ctx is cancelled,
// then waits for all goroutines to finish their current message.
func (a *App) Run(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.retryDispatcher.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.workerPool.Run(ctx)
	}()

	slog.Info("processor started")

	<-ctx.Done()
	slog.Info("shutting down, waiting for workers")
	wg.Wait()
	slog.Info("all workers stopped")
}
