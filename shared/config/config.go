package config

import (
	"strings"

	"github.com/spf13/viper"
)

// Base holds config fields shared by both services.
type Base struct {
	DatabaseURL     string
	RedisAddr       string
	LogLevel        string
	OTelServiceName string
	OTelEndpoint    string
}

// NewViper returns a viper instance with AutomaticEnv and a default log level.
func NewViper() *viper.Viper {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("LOG_LEVEL", "info")
	return v
}

// LoadBase reads the shared required fields from v.
// Returns the field name that is missing, or an empty string on success.
func LoadBase(v *viper.Viper) (Base, string) {
	dbURL := v.GetString("DATABASE_URL")
	redisAddr := v.GetString("REDIS_ADDR")
	switch {
	case dbURL == "":
		return Base{}, "DATABASE_URL"
	case redisAddr == "":
		return Base{}, "REDIS_ADDR"
	}
	return Base{
		DatabaseURL:     dbURL,
		RedisAddr:       redisAddr,
		LogLevel:        v.GetString("LOG_LEVEL"),
		OTelEndpoint:    v.GetString("OTEL_ENDPOINT"),
		OTelServiceName: v.GetString("OTEL_SERVICE_NAME"),
	}, ""
}
