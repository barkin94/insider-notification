package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/barkin/insider-notification/api/internal/config"
	"github.com/barkin/insider-notification/api/internal/consumer"
	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/api/internal/handler"
	"github.com/barkin/insider-notification/api/internal/service"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
)

func main() {
	// --- config & logging ---
	cfg := config.Load()
	initLogger(cfg.LogLevel)

	// TODO: init OTel SDK — Prometheus + OTLP trace exporter (observability task)

	// cancelled on SIGINT / SIGTERM; propagates to all goroutines
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- infrastructure ---
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb, err := sharedredis.NewClient(ctx, cfg.RedisAddr)
	if err != nil {
		slog.Error("connect to redis", "error", err)
		os.Exit(1)
	}

	// --- repositories ---
	notifRepo := db.NewNotificationRepository(pool)
	attemptRepo := db.NewDeliveryAttemptRepository(pool)

	// --- stream publisher & subscriber ---
	pub, err := stream.NewRedisPublisher(rdb)
	if err != nil {
		slog.Error("create stream publisher", "error", err)
		os.Exit(1)
	}

	sub, err := stream.NewRedisSubscriber(rdb, "notify:cg:api")
	if err != nil {
		slog.Error("create stream subscriber", "error", err)
		os.Exit(1)
	}
	defer sub.Close()

	// --- service ---
	svc := service.NewNotificationService(notifRepo, attemptRepo, pub)

	// --- status consumer: reads delivery results from processor and updates DB ---
	statusMsgs, err := stream.Subscribe[stream.NotificationDeliveryResultEvent](ctx, sub, stream.TopicStatus)
	if err != nil {
		slog.Error("subscribe to status stream", "error", err)
		os.Exit(1)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer.NewStatusConsumer(notifRepo, attemptRepo).Run(ctx, statusMsgs)
	}()

	// --- HTTP server ---
	router := handler.NewRouter(handler.Deps{
		Service: svc,
		DB:      pool,
		Redis:   rdb,
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{Addr: addr, Handler: router}

	go func() {
		slog.Info("api server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// --- graceful shutdown ---
	<-ctx.Done()
	slog.Info("shutting down")

	// stop accepting new requests; wait up to 5s for in-flight requests to finish
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	// wait for background goroutines (status consumer) to finish
	wg.Wait()
	slog.Info("all goroutines stopped")
}

func initLogger(level string) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
