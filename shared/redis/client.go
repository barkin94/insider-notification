package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// NewClient creates and pings a Redis client. Panics if unreachable.
func NewClient(ctx context.Context, addr string) *goredis.Client {
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		panic(fmt.Sprintf("redis ping %s: %s", addr, err))
	}
	return client
}
