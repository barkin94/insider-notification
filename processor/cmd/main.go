package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/barkin/insider-notification/processor/internal/app"
	"github.com/barkin/insider-notification/processor/internal/config"
	sharedlogger "github.com/barkin/insider-notification/shared/logger"
	sharedotel "github.com/barkin/insider-notification/shared/otel"
)

func main() {
	cfg := config.Load()
	sharedlogger.Init(cfg.LogLevel)

	if cfg.OTelEnabled {
		otelShutdown := sharedotel.Init(context.Background(), cfg.OTelServiceName, cfg.OTelEndpoint, cfg.LogLevel)
		defer otelShutdown(context.Background())
	}

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
