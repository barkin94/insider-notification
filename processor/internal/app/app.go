package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/barkin/insider-notification/processor/internal/config"
	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/service"
	shareddb "github.com/barkin/insider-notification/shared/db"
	"github.com/barkin/insider-notification/shared/lock"
	"github.com/barkin/insider-notification/shared/model"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/go-redis/redis_rate/v10"
	"time"
)

// App wires and runs the processor service.
type App struct {
	pipeline    *delivery.NotificationDeliveryPipelineWorker
	router      *delivery.PriorityRouter[stream.Result[stream.NotificationReadyEvent]]
	concurrency int
}

// New constructs all dependencies and returns a ready-to-run App.
// The returned cleanup func closes infrastructure (DB, subscriber) and must be deferred by the caller.
func New(ctx context.Context, cfg *config.Config) (*App, func(), error) {
	bundb, err := shareddb.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to postgres: %w", err)
	}

	rdb, err := sharedredis.NewClient(ctx, cfg.RedisAddr)
	if err != nil {
		bundb.Close()
		return nil, nil, fmt.Errorf("connect to redis: %w", err)
	}

	attemptRepo := processordb.NewDeliveryAttemptRepository(bundb)

	pub, err := stream.NewRedisPublisher(rdb)
	if err != nil {
		bundb.Close()
		return nil, nil, fmt.Errorf("create stream publisher: %w", err)
	}

	sub, err := stream.NewRedisSubscriber(rdb, "notify:cg:processor")
	if err != nil {
		bundb.Close()
		return nil, nil, fmt.Errorf("create stream subscriber: %w", err)
	}

	// TODO: PEL reclaim before workers start (priority-router task)

	highMsgs, err := stream.Subscribe[stream.NotificationReadyEvent](ctx, sub, stream.TopicHigh, cfg.OTelServiceName)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("subscribe high: %w", err)
	}
	normalMsgs, err := stream.Subscribe[stream.NotificationReadyEvent](ctx, sub, stream.TopicNormal, cfg.OTelServiceName)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("subscribe normal: %w", err)
	}
	lowMsgs, err := stream.Subscribe[stream.NotificationReadyEvent](ctx, sub, stream.TopicLow, cfg.OTelServiceName)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("subscribe low: %w", err)
	}

	router := delivery.NewPriorityRouter([]delivery.WeightedSource[stream.Result[stream.NotificationReadyEvent]]{
		{Ch: highMsgs, Weight: cfg.HighWeight},
		{Ch: normalMsgs, Weight: cfg.NormalWeight},
		{Ch: lowMsgs, Weight: cfg.LowWeight},
	})

	m, err := service.NewMetrics(rdb)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("init metrics: %w", err)
	}

	pipeline := delivery.NewNotificationDeliveryPipelineWorker(
		pub,
		service.NewNtfnDeliveryClient(cfg.NtfnDeliveryClientURL, cfg.NtfnDeliveryClientTimeout),
		service.NewLimiter(rdb, map[string]redis_rate.Limit{
			model.ChannelSMS:   {Rate: cfg.SMSRatePerSecond, Burst: cfg.SMSBurst, Period: time.Second},
			model.ChannelEmail: {Rate: cfg.EmailRatePerSecond, Burst: cfg.EmailBurst, Period: time.Second},
			model.ChannelPush:  {Rate: cfg.PushRatePerSecond, Burst: cfg.PushBurst, Period: time.Second},
		}),
		lock.NewRedisLocker(rdb),
		attemptRepo,
		m,
	)

	cleanup := func() {
		sub.Close()
		bundb.Close()
	}

	return &App{
		pipeline:    pipeline,
		router:      router,
		concurrency: cfg.WorkerConcurrency,
	}, cleanup, nil
}

// Run starts the scheduler and consumer pool, blocks until ctx is cancelled,
// then waits for all consumers to finish their current message.
func (a *App) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range a.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				result, ok := a.router.Next(ctx)
				if !ok {
					continue
				}
				if result.Err != nil {
					slog.ErrorContext(result.Ctx, "stream read error", "error", result.Err)
					continue
				}
				a.pipeline.Run(result.Ctx, result)
			}
		}()
	}
	slog.Info("processor started", "workers", a.concurrency)

	<-ctx.Done()
	slog.Info("shutting down, waiting for workers")
	wg.Wait()
	slog.Info("all workers stopped")
}
