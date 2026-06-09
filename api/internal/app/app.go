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
	apischeduler "github.com/barkin/insider-notification/api/internal/scheduler"
	"github.com/barkin/insider-notification/api/internal/service"
	handler "github.com/barkin/insider-notification/api/internal/transport/http"
	"github.com/barkin/insider-notification/api/internal/transport/messaging"
	shareddb "github.com/barkin/insider-notification/shared/db"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
)

// App wires and runs the API service.
type App struct {
	server                 *http.Server
	scheduler              *apischeduler.Scheduler
	deliveryResultConsumer *messaging.DeliveryResultConsumer
}

// New constructs all dependencies and returns a ready-to-run App.
// Panics if any infrastructure dependency is unreachable.
func New(ctx context.Context, cfg *config.Config) (*App, func()) {
	bundb := shareddb.Open(cfg.DatabaseURL)
	rdb := sharedredis.NewClient(ctx, cfg.RedisAddr)

	pub := stream.NewRedisPublisher(rdb)
	sub := stream.NewRedisSubscriber(rdb, "notify:cg:api")
	statusMsgs := stream.Subscribe[stream.NotificationDeliveryResultEvent](
		ctx, sub, stream.TopicStatus, cfg.OTelServiceName,
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
		scheduler:              apischeduler.New(notifRepo, pub, cfg.SchedulerInterval),
		deliveryResultConsumer: messaging.NewDeliveryResultConsumer(svc, statusMsgs),
	}, cleanup
}

// Run starts the HTTP server, scheduler, and delivery result consumer, blocks until ctx
// is cancelled, then gracefully shuts down.
func (a *App) Run(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.deliveryResultConsumer.Run(ctx)
	}()

	go a.scheduler.Run(ctx)

	go func() {
		slog.Info("api server starting", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	wg.Wait()
	slog.Info("all goroutines stopped")
}
