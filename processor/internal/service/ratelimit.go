package service

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var defaultLimit = redis_rate.Limit{
	Rate:   100,
	Burst:  120,
	Period: time.Second,
}

// Limiter checks whether a delivery attempt for a given channel is allowed.
// RetryAfter is zero when the request is allowed; positive when rate-limited.
type Limiter interface {
	IsAllowed(ctx context.Context, channel string) (allowed bool, retryAfter time.Duration, err error)
}

type redisLimiter struct {
	limiter       *redis_rate.Limiter
	channelLimits map[string]redis_rate.Limit
}

func NewLimiter(client *redis.Client, channelLimits map[string]redis_rate.Limit) Limiter {
	return &redisLimiter{
		limiter:       redis_rate.NewLimiter(client),
		channelLimits: channelLimits,
	}
}

func (l *redisLimiter) IsAllowed(ctx context.Context, channel string) (bool, time.Duration, error) {
	ctx, span := otel.Tracer("ratelimiter").Start(ctx, "ratelimit.IsAllowed", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetAttributes(attribute.String("ratelimit.channel", channel))

	limit, ok := l.channelLimits[channel]
	if !ok {
		limit = defaultLimit
	}
	key := fmt.Sprintf("ratelimit:{%s}", channel)
	res, err := l.limiter.Allow(ctx, key, limit)
	if err != nil {
		return false, 0, fmt.Errorf("rate limiter: %w", err)
	}
	span.SetAttributes(attribute.Bool("ratelimit.allowed", res.Allowed > 0))
	if res.Allowed > 0 {
		return true, 0, nil
	}
	return false, res.RetryAfter, nil
}
