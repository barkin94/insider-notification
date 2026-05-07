package stream

import (
	"log/slog"

	"github.com/ThreeDotsLabs/watermill"
)

type slogAdapter struct{ logger *slog.Logger }

func NewSlogAdapter(logger *slog.Logger) watermill.LoggerAdapter {
	return &slogAdapter{logger: logger}
}

func (a *slogAdapter) Error(msg string, err error, fields watermill.LogFields) {
	args := logFields(fields)
	if err != nil {
		args = append(args, "error", err)
	}
	a.logger.Error(msg, args...)
}

func (a *slogAdapter) Info(msg string, fields watermill.LogFields) {
	a.logger.Info(msg, logFields(fields)...)
}

func (a *slogAdapter) Debug(msg string, fields watermill.LogFields) {
	a.logger.Debug(msg, logFields(fields)...)
}

func (a *slogAdapter) Trace(msg string, fields watermill.LogFields) {
	// slog has no Trace level; map to Debug
	a.logger.Debug(msg, logFields(fields)...)
}

func (a *slogAdapter) With(fields watermill.LogFields) watermill.LoggerAdapter {
	return &slogAdapter{logger: a.logger.With(logFields(fields)...)}
}

func logFields(fields watermill.LogFields) []any {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return args
}
