package config

import (
	"log/slog"
	"os"
	"time"

	shared "github.com/barkin/insider-notification/shared/config"
)

type Config struct {
	shared.Base
	DatabaseURL       string
	RedisAddr         string
	Port              int
	SchedulerInterval time.Duration
}

func Load() *Config {
	v := shared.NewViper()
	v.SetDefault("PORT", 8080)
	v.SetDefault("SCHEDULER_INTERVAL", "5s")

	databaseURL := v.GetString("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("missing required env var", "var", "DATABASE_URL")
		os.Exit(1)
	}
	redisAddr := v.GetString("REDIS_ADDR")
	if redisAddr == "" {
		slog.Error("missing required env var", "var", "REDIS_ADDR")
		os.Exit(1)
	}

	return &Config{
		Base:              shared.LoadBase(v),
		DatabaseURL:       databaseURL,
		RedisAddr:         redisAddr,
		Port:              v.GetInt("PORT"),
		SchedulerInterval: v.GetDuration("SCHEDULER_INTERVAL"),
	}
}
