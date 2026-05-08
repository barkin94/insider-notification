package lock

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const lockTTL = 60 * time.Second

type RedisLocker struct {
	client *redis.Client
}

func NewRedisLocker(client *redis.Client) *RedisLocker {
	return &RedisLocker{client: client}
}

func (r *RedisLocker) TryLock(ctx context.Context, id string) (bool, error) {
	key := fmt.Sprintf("notify:lock:%s", id)
	return r.client.SetNX(ctx, key, 1, lockTTL).Result()
}

func (r *RedisLocker) Unlock(ctx context.Context, id string) error {
	key := fmt.Sprintf("notify:lock:%s", id)
	return r.client.Del(ctx, key).Err()
}
