package messaging_test

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/barkin94/insider-notification/shared/messaging"
)

type testEvent struct {
	ID       string
	Priority string
}

type testResultEvent struct {
	ID     string
	Status string
}

const (
	topicHigh   = "test:stream:high"
	topicNormal = "test:stream:normal"
	topicLow    = "test:stream:low"
	topicStatus = "test:stream:status"
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

var discardLogger = messaging.NewSlogAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)))

func newPublisher(t *testing.T) messaging.Publisher {
	t.Helper()
	pub, err := redisstream.NewPublisher(redisstream.PublisherConfig{
		Client: redis.NewClient(&redis.Options{Addr: redisAddr}),
	}, discardLogger)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	return messaging.NewPublisher(pub)
}

func newSubscriber(t *testing.T, group string) *redisstream.Subscriber {
	t.Helper()
	sub, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        redis.NewClient(&redis.Options{Addr: redisAddr}),
		ConsumerGroup: group,
	}, discardLogger)
	if err != nil {
		t.Fatalf("new subscriber: %v", err)
	}
	return sub
}

func TestPublisher_routesToCorrectTopic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cases := []struct {
		priority string
		topic    string
	}{
		{"high", topicHigh},
		{"normal", topicNormal},
		{"low", topicLow},
	}

	for _, tc := range cases {
		pub := newPublisher(t)
		sub := newSubscriber(t, "test-cg-"+tc.priority)

		msgs := messaging.Subscribe[testEvent](ctx, sub, tc.topic, "test")

		evt := testEvent{ID: "id-" + tc.priority, Priority: tc.priority}
		if err := pub.Publish(ctx, tc.topic, evt); err != nil {
			t.Fatalf("publish: %v", err)
		}

		result := <-msgs
		if result.Event.ID != evt.ID {
			t.Errorf("got id %s, want %s", result.Event.ID, evt.ID)
		}
		result.Msg.Ack()
	}
}

func TestPublisher_deliveryResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pub := newPublisher(t)
	sub := newSubscriber(t, "test-status-cg")

	msgs := messaging.Subscribe[testResultEvent](ctx, sub, topicStatus, "test")

	evt := testResultEvent{ID: "notif-1", Status: "delivered"}
	if err := pub.Publish(ctx, topicStatus, evt); err != nil {
		t.Fatalf("publish: %v", err)
	}

	result := <-msgs
	if result.Event.ID != evt.ID {
		t.Errorf("got id %s, want %s", result.Event.ID, evt.ID)
	}
	result.Msg.Ack()
}
