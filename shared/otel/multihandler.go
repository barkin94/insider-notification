package otel

import (
	"context"
	"log/slog"
)

type multiHandler []slog.Handler

// NewMultiHandler returns an slog.Handler that forwards every record to all handlers.
func NewMultiHandler(handlers ...slog.Handler) slog.Handler {
	return multiHandler(handlers)
}

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make(multiHandler, len(m))
	for i, h := range m {
		next[i] = h.WithAttrs(attrs)
	}
	return next
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	next := make(multiHandler, len(m))
	for i, h := range m {
		next[i] = h.WithGroup(name)
	}
	return next
}
