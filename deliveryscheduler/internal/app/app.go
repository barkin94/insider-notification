package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/barkin/insider-notification/deliveryscheduler/internal/config"
	"github.com/barkin/insider-notification/deliveryscheduler/internal/db"
	"github.com/barkin/insider-notification/deliveryscheduler/internal/dispatcher"
	"github.com/barkin/insider-notification/deliveryscheduler/internal/transport/messaging"
	sharedbun "github.com/barkin/insider-notification/shared/bun"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
)

// App wires and runs the delivery scheduler service.
type App struct {
	consumer   *messaging.Consumer
	dispatcher *dispatcher.ScheduledNotificationDispatcher
	ticker     *time.Ticker
	wg         sync.WaitGroup
}

// New constructs all dependencies and returns a ready-to-run App.
// The returned cleanup func closes infrastructure and must be deferred by the caller.
func New(ctx context.Context, cfg *config.Config) (*App, func(), error) {
	rdb := sharedredis.NewClient(ctx, cfg.RedisAddr)
	bunDB := sharedbun.Connect(cfg.DatabaseURL)
	repo := db.NewScheduledNotificationRepository(bunDB)

	pub := stream.NewRedisPublisher(rdb)
	sub := stream.NewRedisSubscriber(rdb, "notify:cg:deliveryscheduler")
	msgs := stream.Subscribe[stream.NotificationsScheduledEvent](ctx, sub, stream.TopicNotificationScheduled, cfg.OTelServiceName)

	cleanup := func() {
		_ = sub.Close()
		_ = rdb.Close()
		_ = bunDB.Close()
	}

	return &App{
		consumer:   messaging.NewConsumer(repo, msgs),
		dispatcher: dispatcher.NewScheduledNotificationDispatcher(repo, pub, cfg.DeliverySchedulerBatchSize),
	}, cleanup, nil
}

// Start launches the consumer and dispatcher in the background and returns
// a stop function the caller must invoke to wait for all goroutines to finish.
func (a *App) Start(ctx context.Context) func() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.consumer.Run(ctx)
	}()

	a.wg.Add(1)
	a.ticker = time.NewTicker(1 * time.Second)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case <-ctx.Done():
				a.ticker.Stop()
				return
			case <-a.ticker.C:
				a.dispatcher.Tick(ctx)
			}
		}
	}()

	slog.Info("deliveryscheduler started")
	return a.wg.Wait
}
