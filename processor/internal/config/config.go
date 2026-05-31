package config

import (
	"log/slog"
	"os"
	"time"

	shared "github.com/barkin/insider-notification/shared/config"
)

type Config struct {
	shared.Base
	WorkerConcurrency         int
	NtfnDeliveryClientURL     string
	NtfnDeliveryClientTimeout time.Duration
	SchedulerInterval         time.Duration
	HighWeight                int // HIGH_WEIGHT env var
	NormalWeight              int // NORMAL_WEIGHT env var
	LowWeight                 int // LOW_WEIGHT env var
}

func Load() *Config {
	v := shared.NewViper()
	v.SetDefault("WORKER_CONCURRENCY", 10)
	v.SetDefault("NTFN_DELIVERY_CLIENT_URL", "http://localhost:8080")
	v.SetDefault("NTFN_DELIVERY_CLIENT_TIMEOUT", "10s")
	v.SetDefault("SCHEDULER_INTERVAL", "5s")
	v.SetDefault("HIGH_WEIGHT", 3)
	v.SetDefault("NORMAL_WEIGHT", 2)
	v.SetDefault("LOW_WEIGHT", 1)

	base, missing := shared.LoadBase(v)
	if missing != "" {
		slog.Error("missing required env var", "var", missing)
		os.Exit(1)
	}

	return &Config{
		Base:                      base,
		WorkerConcurrency:         v.GetInt("WORKER_CONCURRENCY"),
		NtfnDeliveryClientURL:     v.GetString("NTFN_DELIVERY_CLIENT_URL"),
		NtfnDeliveryClientTimeout: v.GetDuration("NTFN_DELIVERY_CLIENT_TIMEOUT"),
		SchedulerInterval:         v.GetDuration("SCHEDULER_INTERVAL"),
		HighWeight:                v.GetInt("HIGH_WEIGHT"),
		NormalWeight:              v.GetInt("NORMAL_WEIGHT"),
		LowWeight:                 v.GetInt("LOW_WEIGHT"),
	}
}
