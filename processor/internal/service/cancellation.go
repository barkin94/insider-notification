package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CancellationStore checks whether a notification has been cancelled by the API.
type CancellationStore interface {
	IsCancelled(ctx context.Context, id string) (bool, error)
}

type redisCancellationStore struct{ client *redis.Client }

var _ CancellationStore = (*redisCancellationStore)(nil)

func NewRedisCancellationStore(client *redis.Client) *redisCancellationStore {
	return &redisCancellationStore{client: client}
}

func (r *redisCancellationStore) IsCancelled(ctx context.Context, id string) (bool, error) {
	key := fmt.Sprintf("cancelled:%s", id)
	_, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *redisCancellationStore) MarkCancelled(ctx context.Context, id string) error {
	key := fmt.Sprintf("cancelled:%s", id)
	return r.client.Set(ctx, key, 1, 24*time.Hour).Err()
}
