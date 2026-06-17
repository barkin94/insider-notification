package redis

import (
	"context"
	"fmt"
	"time"

	redisotel "github.com/redis/go-redis/extra/redisotel/v9"
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

	if err := redisotel.InstrumentTracing(client); err != nil {
		panic(err)
	}
	if err := redisotel.InstrumentMetrics(client); err != nil {
		panic(err)
	}

	return client
}
