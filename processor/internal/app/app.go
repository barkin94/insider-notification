package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/barkin/insider-notification/processor/internal/config"
	"github.com/barkin/insider-notification/processor/internal/delivery"
	processordb "github.com/barkin/insider-notification/processor/internal/db"
	processormetrics "github.com/barkin/insider-notification/processor/internal/metrics"
	"github.com/barkin/insider-notification/processor/internal/priorityrouter"
	"github.com/barkin/insider-notification/processor/internal/scheduler"
	"github.com/barkin/insider-notification/processor/internal/service"
	shareddb "github.com/barkin/insider-notification/shared/db"
	"github.com/barkin/insider-notification/shared/lock"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
)

// App wires and runs the processor service.
type App struct {
	scheduler   *scheduler.Scheduler
	worker      *delivery.Worker
	router      *priorityrouter.PriorityRouter[stream.Result[stream.NotificationCreatedEvent]]
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

	highMsgs, err := stream.Subscribe[stream.NotificationCreatedEvent](ctx, sub, stream.TopicHigh, cfg.OTelServiceName)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("subscribe high: %w", err)
	}
	normalMsgs, err := stream.Subscribe[stream.NotificationCreatedEvent](ctx, sub, stream.TopicNormal, cfg.OTelServiceName)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("subscribe normal: %w", err)
	}
	lowMsgs, err := stream.Subscribe[stream.NotificationCreatedEvent](ctx, sub, stream.TopicLow, cfg.OTelServiceName)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("subscribe low: %w", err)
	}

	router := priorityrouter.NewPriorityRouter([]priorityrouter.WeightedSource[stream.Result[stream.NotificationCreatedEvent]]{
		{Ch: highMsgs, Weight: cfg.HighWeight},
		{Ch: normalMsgs, Weight: cfg.NormalWeight},
		{Ch: lowMsgs, Weight: cfg.LowWeight},
	})

	m, err := processormetrics.New(rdb)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("init metrics: %w", err)
	}

	c := delivery.NewWorker(
		pub,
		service.NewDeliveryClient(cfg.WebhookURL, cfg.WebhookTimeout),
		service.NewLimiter(rdb),
		lock.NewRedisLocker(rdb),
		service.NewRedisCancellationStore(rdb),
		attemptRepo,
		m,
	)

	notifReader := processordb.NewNotificationReader(bundb)
	sched := scheduler.New(notifReader, attemptRepo, pub, cfg.SchedulerInterval)

	cleanup := func() {
		sub.Close()
		bundb.Close()
	}

	return &App{
		scheduler:   sched,
		worker:      c,
		router:      router,
		concurrency: cfg.WorkerConcurrency,
	}, cleanup, nil
}

// Run starts the scheduler and consumer pool, blocks until ctx is cancelled,
// then waits for all consumers to finish their current message.
func (a *App) Run(ctx context.Context) {
	go a.scheduler.Run(ctx)

	var wg sync.WaitGroup
	for range a.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.worker.Run(ctx, a.router)
		}()
	}
	slog.Info("processor started", "workers", a.concurrency)

	<-ctx.Done()
	slog.Info("shutting down, waiting for workers")
	wg.Wait()
	slog.Info("all workers stopped")
}
