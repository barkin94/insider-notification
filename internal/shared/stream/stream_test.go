package stream_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/barkin/insider-notification/internal/shared/stream"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testClient *redis.Client

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
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			log.Printf("terminate redis container: %v", err)
		}
	}()

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		log.Fatalf("get redis endpoint: %v", err)
	}

	testClient = redis.NewClient(&redis.Options{Addr: addr})
	defer testClient.Close()

	os.Exit(m.Run())
}

func flushRedis(t *testing.T) {
	t.Helper()
	if err := testClient.FlushAll(context.Background()).Err(); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
}

func newProducer() stream.Producer {
	return stream.NewProducer(testClient)
}

func newConsumer(t *testing.T, group, name string) stream.Consumer {
	t.Helper()
	c, err := stream.NewConsumer(context.Background(), testClient, group, name)
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	return c
}

func TestProducer_Publish_routesToCorrectStream(t *testing.T) {
	flushRedis(t)
	ctx := context.Background()
	p := newProducer()

	cases := []struct {
		priority string
		stream   string
	}{
		{"high", "notify:stream:high"},
		{"normal", "notify:stream:normal"},
		{"low", "notify:stream:low"},
	}

	for _, tc := range cases {
		msg := stream.PriorityMessage{
			NotificationID: "id-" + tc.priority,
			Channel:        "sms",
			Recipient:      "+1555",
			Content:        "hello",
			Priority:       tc.priority,
			AttemptNumber:  1,
			MaxAttempts:    3,
			Metadata:       "{}",
		}
		if err := p.Publish(ctx, tc.priority, msg); err != nil {
			t.Fatalf("Publish(%s): %v", tc.priority, err)
		}
		length := testClient.XLen(ctx, tc.stream).Val()
		if length != 1 {
			t.Errorf("stream %s: expected 1 message, got %d", tc.stream, length)
		}
	}
}

func TestProducer_PublishStatus(t *testing.T) {
	flushRedis(t)
	ctx := context.Background()
	p := newProducer()

	msg := stream.StatusMessage{
		NotificationID: "notif-1",
		Status:         "delivered",
		AttemptNumber:  1,
		HTTPStatusCode: 200,
		LatencyMS:      100,
		UpdatedAt:      time.Now().Format(time.RFC3339),
	}
	if err := p.PublishStatus(ctx, msg); err != nil {
		t.Fatalf("PublishStatus: %v", err)
	}
	if n := testClient.XLen(ctx, "notify:stream:status").Val(); n != 1 {
		t.Errorf("status stream: expected 1 message, got %d", n)
	}
}

func TestConsumer_ReadPriority_order(t *testing.T) {
	flushRedis(t)
	ctx := context.Background()
	p := newProducer()
	c := newConsumer(t, "test:cg", "worker-1")

	for _, prio := range []string{"low", "normal", "high"} {
		if err := p.Publish(ctx, prio, stream.PriorityMessage{
			NotificationID: "id-" + prio,
			Channel:        "email",
			Recipient:      "a@b.com",
			Content:        "msg",
			Priority:       prio,
			AttemptNumber:  1,
			MaxAttempts:    3,
			Metadata:       "{}",
		}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	expected := []string{"high", "normal", "low"}
	for _, want := range expected {
		msg, id, err := c.ReadPriority(ctx)
		if err != nil {
			t.Fatalf("ReadPriority: %v", err)
		}
		if msg == nil {
			t.Fatal("expected message, got nil")
		}
		if msg.Priority != want {
			t.Errorf("expected priority %s, got %s", want, msg.Priority)
		}
		if err := c.Ack(ctx, "notify:stream:"+want, id); err != nil {
			t.Fatalf("Ack: %v", err)
		}
	}
}

func TestConsumer_Ack(t *testing.T) {
	flushRedis(t)
	ctx := context.Background()
	p := newProducer()
	c := newConsumer(t, "ack:cg", "worker-1")

	if err := p.Publish(ctx, "high", stream.PriorityMessage{
		NotificationID: "ack-test",
		Channel:        "sms",
		Recipient:      "+1",
		Content:        "hi",
		Priority:       "high",
		AttemptNumber:  1,
		MaxAttempts:    3,
		Metadata:       "{}",
	}); err != nil {
		t.Fatal(err)
	}

	_, id, err := c.ReadPriority(ctx)
	if err != nil {
		t.Fatalf("ReadPriority: %v", err)
	}

	pending := testClient.XPending(ctx, "notify:stream:high", "ack:cg").Val()
	if pending.Count != 1 {
		t.Errorf("expected 1 pending before ack, got %d", pending.Count)
	}

	if err := c.Ack(ctx, "notify:stream:high", id); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	pending = testClient.XPending(ctx, "notify:stream:high", "ack:cg").Val()
	if pending.Count != 0 {
		t.Errorf("expected 0 pending after ack, got %d", pending.Count)
	}
}

func TestConsumer_ReclaimStale(t *testing.T) {
	flushRedis(t)
	ctx := context.Background()
	p := newProducer()

	// First consumer reads but does not ack
	c1 := newConsumer(t, "reclaim:cg", "worker-1")
	if err := p.Publish(ctx, "high", stream.PriorityMessage{
		NotificationID: "stale-test",
		Channel:        "push",
		Recipient:      "token",
		Content:        "ping",
		Priority:       "high",
		AttemptNumber:  1,
		MaxAttempts:    3,
		Metadata:       "{}",
	}); err != nil {
		t.Fatal(err)
	}

	_, _, err := c1.ReadPriority(ctx)
	if err != nil {
		t.Fatalf("ReadPriority: %v", err)
	}

	// Second consumer reclaims with zero minIdle (any unacked message qualifies)
	c2 := newConsumer(t, "reclaim:cg", "worker-2")
	if err := c2.ReclaimStale(ctx, "notify:stream:high", 0); err != nil {
		t.Fatalf("ReclaimStale: %v", err)
	}

	pending := testClient.XPending(ctx, "notify:stream:high", "reclaim:cg").Val()
	if pending.Count != 1 {
		t.Errorf("expected 1 pending entry after reclaim, got %d", pending.Count)
	}
	if pending.Consumers["worker-2"] != 1 {
		t.Errorf("expected worker-2 to own the message, got %+v", pending.Consumers)
	}
}

func TestConsumer_groupCreatedIfAbsent(t *testing.T) {
	flushRedis(t)
	ctx := context.Background()

	// NewConsumer on a fresh Redis should succeed without error
	_, err := stream.NewConsumer(ctx, testClient, "fresh:cg", "worker-1")
	if err != nil {
		t.Fatalf("NewConsumer on fresh Redis: %v", err)
	}

	// Second call with the same group should also succeed (BUSYGROUP swallowed)
	_, err = stream.NewConsumer(ctx, testClient, "fresh:cg", "worker-2")
	if err != nil {
		t.Fatalf("NewConsumer with existing group: %v", err)
	}
}
