package config

import (
	"log/slog"
	"os"

	shared "github.com/barkin/insider-notification/shared/config"
)

type Config struct {
	shared.Base
	WorkerConcurrency int
	WebhookURL        string
}

func Load() *Config {
	v := shared.NewViper()
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
		WorkerConcurrency: v.GetInt("WORKER_CONCURRENCY"),
		WebhookURL:        webhookURL,
	}
}
