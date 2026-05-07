package config

import (
	"fmt"
	"log"
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
		fmt.Fprintf(os.Stderr, "config error: %s is required\n", missing)
		log.Fatalf("missing required env var: %s", missing)
	}

	webhookURL := v.GetString("WEBHOOK_URL")
	if webhookURL == "" {
		fmt.Fprintf(os.Stderr, "config error: WEBHOOK_URL is required\n")
		log.Fatal("missing required env var: WEBHOOK_URL")
	}

	return &Config{
		Base:              base,
		MetricsPort:       v.GetInt("PROCESSOR_METRICS_PORT"),
		WorkerConcurrency: v.GetInt("WORKER_CONCURRENCY"),
		WebhookURL:        webhookURL,
	}
}
