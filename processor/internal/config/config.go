package config

import (
	"log/slog"
	"os"

	shared "github.com/barkin/insider-notification/internal/shared/config"
)

type Config struct {
	shared.Base
	MetricsPort       int
	WorkerConcurrency int
	WebhookURL        string
}

func Load() *Config {
	v := shared.NewViper()
	v.SetDefault("PROCESSOR_METRICS_PORT", 8081)
	v.SetDefault("WORKER_CONCURRENCY", 10)

	base, missing := shared.LoadBase(v)
	if missing != "" {
		slog.Error("missing required env var", "var", missing)
		os.Exit(1)
	}

	webhookURL := v.GetString("WEBHOOK_URL")
	if webhookURL == "" {
		slog.Error("missing required env var", "var", "WEBHOOK_URL")
		os.Exit(1)
	}

	return &Config{
		Base:              base,
		MetricsPort:       v.GetInt("PROCESSOR_METRICS_PORT"),
		WorkerConcurrency: v.GetInt("WORKER_CONCURRENCY"),
		WebhookURL:        webhookURL,
	}
}
