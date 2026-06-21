package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-redis/redis_rate/v10"

	"github.com/barkin/insider-notification/processor/internal/config"
	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/service"
	"github.com/barkin/insider-notification/processor/internal/transport/messaging"
	"github.com/barkin/insider-notification/shared/lock"
	apipub "github.com/barkin/insider-notification/api/public"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	stream "github.com/barkin/insider-notification/shared/messaging"
)

// App wires and runs the processor service.
type App struct {
	workerPool *delivery.NotificationDeliveryWorkerPool
	wg         sync.WaitGroup
}

// New constructs all dependencies and returns a ready-to-run App.
// The returned cleanup func closes infrastructure and must be deferred by the caller.
func New(ctx context.Context, cfg *config.Config) (*App, func(), error) {
	rdb := sharedredis.NewClient(ctx, cfg.RedisAddr)

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
			string(apipub.ChannelSMS):   {Rate: cfg.SMSRatePerSecond, Burst: cfg.SMSBurst, Period: time.Second},
			string(apipub.ChannelEmail): {Rate: cfg.EmailRatePerSecond, Burst: cfg.EmailBurst, Period: time.Second},
			string(apipub.ChannelPush):  {Rate: cfg.PushRatePerSecond, Burst: cfg.PushBurst, Period: time.Second},
		}),
		lock.NewRedisLocker(rdb),
		m,
	)

	cleanup := func() {
		_ = sub.Close()
		_ = rdb.Close()
	}

	router := messaging.NewNotificationRouter(ctx, sub, cfg.OTelServiceName, cfg.HighWeight, cfg.NormalWeight, cfg.LowWeight)

	return &App{
		workerPool: delivery.NewNotificationDeliveryWorkerPool(router, pipeline, cfg.WorkerConcurrency),
	}, cleanup, nil
}

// Start launches the worker pool in the background and returns a stop function
// the caller must invoke to wait for all workers to finish their current message.
func (a *App) Start(ctx context.Context) func() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.workerPool.Run(ctx)
	}()

	slog.Info("processor started")
	return a.wg.Wait
}
