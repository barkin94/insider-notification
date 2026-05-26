package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/barkin/insider-notification/processor/internal/app"
	"github.com/barkin/insider-notification/processor/internal/config"
	sharedotel "github.com/barkin/insider-notification/shared/otel"
)

func main() {
	cfg := config.Load()

	otelShutdown, err := sharedotel.Init(context.Background(), cfg.OTelServiceName, cfg.OTelEndpoint)
	if err != nil {
		slog.Error("init otel", "error", err)
		os.Exit(1)
	}
	defer otelShutdown(context.Background())
	sharedotel.InitLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, cleanup, err := app.New(ctx, cfg)
	if err != nil {
		slog.Error("init app", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	a.Run(ctx)
}
