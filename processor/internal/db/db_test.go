package db_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

var redisAddr string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("6379/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		log.Printf("Redis container unavailable, skipping db integration tests: %v", err)
		os.Exit(m.Run())
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			log.Printf("terminate redis container: %v", err)
		}
	}()

	redisAddr, err = container.Endpoint(ctx, "")
	if err != nil {
		log.Fatalf("get redis endpoint: %v", err)
	}

	os.Exit(m.Run())
}

func requireRedis(t *testing.T) {
	t.Helper()
	if redisAddr == "" {
		t.Skip("Redis not available (Docker not running)")
	}
}

func newRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: redisAddr})
}
