package config

import (
	"strings"

	"github.com/spf13/viper"
)

// Base holds config fields shared across all services.
type Base struct {
	LogLevel        string
	OTelEnabled     bool
	OTelServiceName string
	OTelEndpoint    string
}

// NewViper returns a viper instance with AutomaticEnv and a default log level.
func NewViper() *viper.Viper {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("OTEL_ENABLED", false)
	return v
}

// LoadBase reads the shared fields from v.
func LoadBase(v *viper.Viper) Base {
	return Base{
		LogLevel:        v.GetString("LOG_LEVEL"),
		OTelEnabled:     v.GetBool("OTEL_ENABLED"),
		OTelEndpoint:    v.GetString("OTEL_ENDPOINT"),
		OTelServiceName: v.GetString("OTEL_SERVICE_NAME"),
	}
}
