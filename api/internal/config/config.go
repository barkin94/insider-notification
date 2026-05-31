package config

import (
	"log/slog"
	"os"
	"time"

	shared "github.com/barkin/insider-notification/shared/config"
)

type Config struct {
	shared.Base
	Port              int
	SchedulerInterval time.Duration
}

func Load() *Config {
	v := shared.NewViper()
	v.SetDefault("PORT", 8080)
	v.SetDefault("SCHEDULER_INTERVAL", "5s")

	base, missing := shared.LoadBase(v)
	if missing != "" {
		slog.Error("missing required env var", "var", missing)
		os.Exit(1)
	}

	return &Config{
		Base:              base,
		Port:              v.GetInt("PORT"),
		SchedulerInterval: v.GetDuration("SCHEDULER_INTERVAL"),
	}
}
