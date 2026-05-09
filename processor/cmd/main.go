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

	"github.com/barkin/insider-notification/processor/internal/config"
	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/ratelimit"
	"github.com/barkin/insider-notification/processor/internal/worker"
	"github.com/barkin/insider-notification/shared/lock"
	sharedotel "github.com/barkin/insider-notification/shared/otel"
	sharedredis "github.com/barkin/insider-notification/shared/redis"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/bridges/otelslog"
)

func main() {
	// --- config & logging ---
	cfg := config.Load()
	initLogger(cfg.LogLevel, "processor")

	// --- OTel SDK: traces (OTLP gRPC) + metrics (Prometheus) ---
	otelShutdown, err := sharedotel.Init(context.Background(), "processor", cfg.OTelEndpoint)
	if err != nil {
		slog.Error("init otel", "error", err)
		os.Exit(1)
	}
	defer otelShutdown(context.Background())

	// cancelled on SIGINT / SIGTERM; propagates to all goroutines
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- infrastructure ---
	rdb, err := sharedredis.NewClient(ctx, cfg.RedisAddr)
	if err != nil {
		slog.Error("connect to redis", "error", err)
		os.Exit(1)
	}

	// --- stream publisher & subscriber ---
	pub, err := stream.NewRedisPublisher(rdb)
	if err != nil {
		slog.Error("create stream publisher", "error", err)
		os.Exit(1)
	}

	sub, err := stream.NewRedisSubscriber(rdb, "notify:cg:processor")
	if err != nil {
		slog.Error("create stream subscriber", "error", err)
		os.Exit(1)
	}
	defer sub.Close()

	// TODO: PEL reclaim before workers start (priority-router task)

	// --- subscribe to all three priority topics; fan-in to one channel ---
	highMsgs, err := stream.Subscribe[stream.NotificationCreatedEvent](ctx, sub, stream.TopicHigh)
	if err != nil {
		slog.Error("subscribe high", "error", err)
		os.Exit(1)
	}
	normalMsgs, err := stream.Subscribe[stream.NotificationCreatedEvent](ctx, sub, stream.TopicNormal)
	if err != nil {
		slog.Error("subscribe normal", "error", err)
		os.Exit(1)
	}
	lowMsgs, err := stream.Subscribe[stream.NotificationCreatedEvent](ctx, sub, stream.TopicLow)
	if err != nil {
		slog.Error("subscribe low", "error", err)
		os.Exit(1)
	}
	msgs := fanIn(ctx, highMsgs, normalMsgs, lowMsgs)

	// --- worker dependencies ---
	limiter := ratelimit.NewLimiter(rdb)
	deliveryClient := delivery.NewClient(cfg.WebhookURL, 10*time.Second)
	locker := lock.NewRedisLocker(rdb)
	canceller := worker.NewRedisCancellationStore(rdb)

	w := worker.NewWorker(pub, deliveryClient, limiter, locker, canceller)

	// --- start worker pool ---
	var wg sync.WaitGroup
	for range cfg.WorkerConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Run(ctx, msgs)
		}()
	}
	slog.Info("processor started", "workers", cfg.WorkerConcurrency)

	// --- metrics server: Prometheus /metrics on cfg.MetricsPort ---
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsAddr := fmt.Sprintf(":%d", cfg.MetricsPort)
	metricsServer := &http.Server{Addr: metricsAddr, Handler: metricsMux}
	go func() {
		slog.Info("metrics server starting", "addr", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	// --- graceful shutdown: wait for all workers to finish current message ---
	<-ctx.Done()
	slog.Info("shutting down, waiting for workers")
	wg.Wait()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metricsServer.Shutdown(shutdownCtx)
	slog.Info("all workers stopped")
}

func fanIn(ctx context.Context, channels ...<-chan stream.Result[stream.NotificationCreatedEvent]) <-chan stream.Result[stream.NotificationCreatedEvent] {
	out := make(chan stream.Result[stream.NotificationCreatedEvent])
	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func(c <-chan stream.Result[stream.NotificationCreatedEvent]) {
			defer wg.Done()
			for {
				select {
				case msg, ok := <-c:
					if !ok {
						return
					}
					select {
					case out <- msg:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func initLogger(level, serviceName string) {
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
	opts := &slog.HandlerOptions{Level: l}
	slog.SetDefault(slog.New(sharedotel.NewMultiHandler(
		slog.NewJSONHandler(os.Stdout, opts),
		otelslog.NewHandler(serviceName),
	)))
}
