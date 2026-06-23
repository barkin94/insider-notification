package config

import (
	"log/slog"
	"os"

	shared "github.com/barkin94/insider-notification/shared/config"
)

type Config struct {
	shared.Base
	DatabaseURL                string
	NATSAddr                   string
	DeliverySchedulerBatchSize int
}

func Load() *Config {
	v := shared.NewViper()
	v.SetDefault("DELIVERY_SCHEDULER_BATCH_SIZE", 100)
	v.SetDefault("OTEL_SERVICE_NAME", "deliveryscheduler")

	databaseURL := v.GetString("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("missing required env var", "var", "DATABASE_URL")
		os.Exit(1)
	}
	natsAddr := v.GetString("NATS_ADDR")
	if natsAddr == "" {
		slog.Error("missing required env var", "var", "NATS_ADDR")
		os.Exit(1)
	}

	return &Config{
		Base:                       shared.LoadBase(v),
		DatabaseURL:                databaseURL,
		NATSAddr:                   natsAddr,
		DeliverySchedulerBatchSize: v.GetInt("DELIVERY_SCHEDULER_BATCH_SIZE"),
	}
}
