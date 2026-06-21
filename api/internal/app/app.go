package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/barkin/insider-notification/api/internal/config"
	"github.com/barkin/insider-notification/api/internal/repository/postgres"
	"github.com/barkin/insider-notification/api/internal/service"
	handler "github.com/barkin/insider-notification/api/internal/transport/http"
	"github.com/barkin/insider-notification/api/internal/transport/messaging"
	sharedbun "github.com/barkin/insider-notification/shared/bun"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
)

// App wires and runs the API service.
type App struct {
	server                 *http.Server
	deliveryResultConsumer *messaging.DeliveryResultConsumer
	scheduledDueConsumer   *messaging.ScheduledDueConsumer
	wg                     sync.WaitGroup
}

// New constructs all dependencies and returns a ready-to-run App.
// Panics if any infrastructure dependency is unreachable.
func New(ctx context.Context, cfg *config.Config) (*App, func()) {
	bundb := sharedbun.Connect(cfg.DatabaseURL)
	rdb := sharedredis.NewClient(ctx, cfg.RedisAddr)

	pub := stream.NewRedisPublisher(rdb)
	sub := stream.NewRedisSubscriber(rdb, "notify:cg:api")
	statusMsgs := stream.Subscribe[stream.NotificationDeliveryResultEvent](
		ctx, sub, stream.TopicStatus, cfg.OTelServiceName,
	)
	scheduledDueMsgs := stream.Subscribe[stream.ScheduledNotificationDueEvent](
		ctx, sub, stream.TopicScheduledNotificationDue, cfg.OTelServiceName,
	)

	notifRepo := postgres.NewNotificationRepository(bundb)
	svc := service.NewNotificationService(notifRepo, pub)
	router := handler.NewRouter(handler.Deps{Service: svc, DB: bundb, Redis: rdb})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 30 * time.Second,
	}

	cleanup := func() {
		_ = sub.Close()
		_ = rdb.Close()
		_ = bundb.Close()
	}

	return &App{
		server:                 srv,
		deliveryResultConsumer: messaging.NewDeliveryResultConsumer(svc, statusMsgs),
		scheduledDueConsumer:   messaging.NewScheduledDueConsumer(notifRepo, pub, scheduledDueMsgs),
	}, cleanup
}

// Start launches the HTTP server, scheduler, and consumers in the background.
// It returns a stop function that the caller must invoke to drain all goroutines gracefully.
func (a *App) Start(ctx context.Context) func(context.Context) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.deliveryResultConsumer.Run(ctx)
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.scheduledDueConsumer.Run(ctx)
	}()

	go func() {
		slog.Info("api server starting", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	return func(ctx context.Context) {
		if err := a.server.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
		a.wg.Wait()
	}
}
