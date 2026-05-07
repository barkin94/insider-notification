package config

import (
	"fmt"
	"log"
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
		fmt.Fprintf(os.Stderr, "config error: %s is required\n", missing)
		log.Fatalf("missing required env var: %s", missing)
	}

	return &Config{
		Base: base,
		Port: v.GetInt("PORT"),
	}
}
