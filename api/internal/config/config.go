package config

import (
	"log/slog"
	"os"

	shared "github.com/barkin/insider-notification/internal/shared/config"
)

type Config struct {
	shared.Base
	Port int
}

func Load() *Config {
	v := shared.NewViper()
	v.SetDefault("PORT", 8080)

	base, missing := shared.LoadBase(v)
	if missing != "" {
		slog.Error("missing required env var", "var", missing)
		os.Exit(1)
	}

	return &Config{
		Base: base,
		Port: v.GetInt("PORT"),
	}
}
