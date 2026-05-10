package ratelimit_test

import (
	"context"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/barkin/insider-notification/processor/internal/worker/ratelimit"
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
		log.Fatalf("start redis container: %v", err)
	}
	defer container.Terminate(ctx) //nolint:errcheck

	redisAddr, err = container.Endpoint(ctx, "")
	if err != nil {
		log.Fatalf("get redis endpoint: %v", err)
	}

	os.Exit(m.Run())
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: redisAddr})
}

func TestLimiter_allows(t *testing.T) {
	ctx := context.Background()
	limiter := ratelimit.NewLimiter(newClient())

	for i := 0; i < 100; i++ {
		ok, err := limiter.Allow(ctx, "sms")
		if err != nil {
			t.Fatalf("Allow error at i=%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("expected allowed at i=%d", i)
		}
	}
}

func TestLimiter_throttles(t *testing.T) {
	ctx := context.Background()
	limiter := ratelimit.NewLimiter(newClient())

	denied := 0
	for i := 0; i < 500; i++ {
		ok, err := limiter.Allow(ctx, "email")
		if err != nil {
			t.Fatalf("Allow error: %v", err)
		}
		if !ok {
			denied++
		}
	}
	if denied == 0 {
		t.Error("expected some requests to be denied when burst exceeded")
	}
}

func TestLimiter_refills(t *testing.T) {
	ctx := context.Background()
	limiter := ratelimit.NewLimiter(newClient())

	for i := 0; i < 500; i++ {
		limiter.Allow(ctx, "push") //nolint:errcheck
	}

	ok, err := limiter.Allow(ctx, "push")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected denied after bucket exhausted")
	}

	time.Sleep(100 * time.Millisecond)

	ok, err = limiter.Allow(ctx, "push")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected allowed after refill window")
	}
}

func TestLimiter_atomic(t *testing.T) {
	ctx := context.Background()
	limiter := ratelimit.NewLimiter(newClient())

	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := limiter.Allow(ctx, "concurrent")
			if err != nil {
				t.Errorf("Allow error: %v", err)
				return
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if allowed.Load() != 50 {
		t.Errorf("expected 50 allowed, got %d", allowed.Load())
	}
}
