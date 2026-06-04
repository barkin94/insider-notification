package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/barkin/insider-notification/api/internal/config"
	"github.com/barkin/insider-notification/api/internal/consumer"
	"github.com/barkin/insider-notification/api/internal/db/repos"
	"github.com/barkin/insider-notification/api/internal/handler"
	apischeduler "github.com/barkin/insider-notification/api/internal/scheduler"
	"github.com/barkin/insider-notification/api/internal/service"
	shareddb "github.com/barkin/insider-notification/shared/db"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
)

// App wires and runs the API service.
type App struct {
	server     *http.Server
	scheduler  *apischeduler.Scheduler
	consumer   *consumer.StatusConsumer
	statusMsgs <-chan stream.Result[stream.NotificationDeliveryResultEvent]
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

	pub, err := stream.NewRedisPublisher(rdb)
	if err != nil {
		bundb.Close()
		return nil, nil, fmt.Errorf("create stream publisher: %w", err)
	}

	sub, err := stream.NewRedisSubscriber(rdb, "notify:cg:api")
	if err != nil {
		bundb.Close()
		return nil, nil, fmt.Errorf("create stream subscriber: %w", err)
	}

	statusMsgs, err := stream.Subscribe[stream.NotificationDeliveryResultEvent](
		ctx, sub, stream.TopicStatus, cfg.OTelServiceName,
	)
	if err != nil {
		sub.Close()
		bundb.Close()
		return nil, nil, fmt.Errorf("subscribe to status stream: %w", err)
	}

	notifRepo := repos.NewNotificationRepository(bundb)
	svc := service.NewNotificationService(notifRepo, pub)
	router := handler.NewRouter(handler.Deps{Service: svc, DB: bundb, Redis: rdb})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	cleanup := func() {
		sub.Close()
		bundb.Close()
	}

	return &App{
		server:     srv,
		scheduler:  apischeduler.New(notifRepo, pub, cfg.SchedulerInterval),
		consumer:   consumer.NewStatusConsumer(notifRepo),
		statusMsgs: statusMsgs,
	}, cleanup, nil
}

// Run starts the HTTP server, scheduler, and status consumer, blocks until ctx
// is cancelled, then gracefully shuts down.
func (a *App) Run(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.consumer.Run(ctx, a.statusMsgs)
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
