package config

import (
	"log/slog"
	"os"
	"time"

	shared "github.com/barkin/insider-notification/shared/config"
)

type Config struct {
	shared.Base
	RetryDispatchInterval  time.Duration
	RetryDispatchBatchSize int
}

func Load() *Config {
	v := shared.NewViper()
	v.SetDefault("RETRY_DISPATCH_INTERVAL", "1s")
	v.SetDefault("RETRY_DISPATCH_BATCH_SIZE", 100)
	v.SetDefault("OTEL_SERVICE_NAME", "retryscheduler")

	redisAddr := v.GetString("REDIS_ADDR")
	if redisAddr == "" {
		slog.Error("missing required env var", "var", "REDIS_ADDR")
		os.Exit(1)
	}
	databaseURL := v.GetString("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("missing required env var", "var", "DATABASE_URL")
		os.Exit(1)
	}

	return &Config{
		Base: shared.Base{
			DatabaseURL:     databaseURL,
			RedisAddr:       redisAddr,
			LogLevel:        v.GetString("LOG_LEVEL"),
			OTelEnabled:     v.GetBool("OTEL_ENABLED"),
			OTelEndpoint:    v.GetString("OTEL_ENDPOINT"),
			OTelServiceName: v.GetString("OTEL_SERVICE_NAME"),
		},
		RetryDispatchInterval:  v.GetDuration("RETRY_DISPATCH_INTERVAL"),
		RetryDispatchBatchSize: v.GetInt("RETRY_DISPATCH_BATCH_SIZE"),
	}
}
