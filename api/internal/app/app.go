package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/barkin94/insider-notification/api/internal/config"
	"github.com/barkin94/insider-notification/api/internal/db/postgres"
	"github.com/barkin94/insider-notification/api/internal/service"
	handler "github.com/barkin94/insider-notification/api/internal/transport/http"
	"github.com/barkin94/insider-notification/api/internal/transport/messaging"
	dspub "github.com/barkin94/insider-notification/deliveryscheduler/public"
	processorpub "github.com/barkin94/insider-notification/processor/public"
	sharedbun "github.com/barkin94/insider-notification/shared/bun"
	natsmsg "github.com/barkin94/insider-notification/shared/messaging/nats"
	sharedredis "github.com/barkin94/insider-notification/shared/redis"
)

const notificationStream = "NOTIFICATIONS"

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

	natsHandle := natsmsg.NewHandle(cfg.NATSAddr)
	if err := natsmsg.EnsureStream(natsHandle, notificationStream, []string{"notify.>"}); err != nil {
		panic("ensure nats stream: " + err.Error())
	}
	pub := natsmsg.NewPublisher(natsHandle)

	statusMsgs := natsmsg.Subscribe[processorpub.NotificationDeliveryResultEvent](
		ctx, natsHandle, processorpub.TopicStatus, "api-status", cfg.OTelServiceName, 0,
	)
	scheduledDueMsgs := natsmsg.Subscribe[dspub.ScheduledNotificationDueEvent](
		ctx, natsHandle, dspub.TopicScheduledNotificationDue, "api-scheduled-due", cfg.OTelServiceName, 0,
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
		natsHandle.Close()
		_ = rdb.Close()
		_ = bundb.Close()
	}

	return &App{
		server:                 srv,
		deliveryResultConsumer: messaging.NewDeliveryResultConsumer(svc, statusMsgs),
		scheduledDueConsumer:   messaging.NewScheduledDueConsumer(notifRepo, pub, scheduledDueMsgs),
	}, cleanup
}

// Start launches the HTTP server and consumers in the background.
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
