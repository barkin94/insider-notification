// @title           Insider Notification API
// @version         1.0
// @description     Notification delivery service — create, list, and cancel notifications.
// @host            localhost:8080
// @BasePath        /api/v1
// @schemes         http

package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/barkin/insider-notification/api/internal/app"
	"github.com/barkin/insider-notification/api/internal/config"
	sharedlogger "github.com/barkin/insider-notification/shared/logger"
	sharedotel "github.com/barkin/insider-notification/shared/otel"
)

func main() {
	cfg := config.Load()
	sharedlogger.Init(cfg.LogLevel)

	if cfg.OTelEnabled {
		otelShutdown := sharedotel.Init(context.Background(), cfg.OTelServiceName, cfg.OTelEndpoint, cfg.LogLevel)
		defer otelShutdown(context.Background()) //nolint:errcheck
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, cleanup := app.New(ctx, cfg)
	defer cleanup()

	a.Run(ctx)
}
