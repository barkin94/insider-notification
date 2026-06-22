package lock

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const lockTTL = 10 * time.Second

type RedisLocker struct {
	client *redis.Client
	tracer trace.Tracer
}

func NewRedisLocker(client *redis.Client) *RedisLocker {
	return &RedisLocker{
		client: client,
		tracer: otel.Tracer("lock"),
	}
}

func (r *RedisLocker) TryLock(ctx context.Context, id string) (bool, error) {
	ctx, span := r.tracer.Start(ctx, "lock.TryLock", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetAttributes(attribute.String("lock.id", id))

	key := fmt.Sprintf("notify:lock:%s", id)
	acquired, err := r.client.SetNX(ctx, key, 1, lockTTL).Result()
	if err != nil {
		span.RecordError(err)
		return false, err
	}
	span.SetAttributes(attribute.Bool("lock.acquired", acquired))
	return acquired, nil
}

func (r *RedisLocker) Unlock(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "lock.Unlock", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetAttributes(attribute.String("lock.id", id))

	key := fmt.Sprintf("notify:lock:%s", id)
	if err := r.client.Del(ctx, key).Err(); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}
