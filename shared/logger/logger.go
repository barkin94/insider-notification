package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the global slog logger to write to stderr at the given level.
// Call this before otel.Init; otel.Init will override it with the OTel bridge when enabled.
func Init(logLevel string) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: ParseLevel(logLevel)})))
}

func ParseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
