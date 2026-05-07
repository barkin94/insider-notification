package stream_test

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/barkin/insider-notification/internal/shared/stream"
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

var discardLogger = stream.NewSlogAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)))

func newPublisher(t *testing.T) *stream.Publisher {
	t.Helper()
	pub, err := redisstream.NewPublisher(redisstream.PublisherConfig{
		Client: redis.NewClient(&redis.Options{Addr: redisAddr}),
	}, discardLogger)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	return stream.NewPublisher(pub)
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
		{"high", stream.TopicHigh},
		{"normal", stream.TopicNormal},
		{"low", stream.TopicLow},
	}

	for _, tc := range cases {
		pub := newPublisher(t)
		sub := newSubscriber(t, "test-cg-"+tc.priority)

		msgs, err := stream.Subscribe[stream.NotificationCreatedEvent](ctx, sub, tc.topic)
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}

		evt := stream.NotificationCreatedEvent{
			NotificationID: "id-" + tc.priority,
			Channel:        "sms",
			Recipient:      "+1555",
			Content:        "hello",
			Priority:       tc.priority,
			AttemptNumber:  1,
			MaxAttempts:    3,
			Metadata:       "{}",
		}
		if err := pub.Publish(ctx, tc.topic, evt); err != nil {
			t.Fatalf("publish: %v", err)
		}

		result := <-msgs
		if result.Err != nil {
			t.Fatalf("receive: %v", result.Err)
		}
		if result.Event.NotificationID != evt.NotificationID {
			t.Errorf("got id %s, want %s", result.Event.NotificationID, evt.NotificationID)
		}
		result.Msg.Ack()
	}
}

func TestPublisher_deliveryResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pub := newPublisher(t)
	sub := newSubscriber(t, "test-status-cg")

	msgs, err := stream.Subscribe[stream.NotificationDeliveryResultEvent](ctx, sub, stream.TopicStatus)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	evt := stream.NotificationDeliveryResultEvent{
		NotificationID: "notif-1",
		Status:         "delivered",
		AttemptNumber:  1,
		HTTPStatusCode: 200,
		LatencyMS:      100,
		UpdatedAt:      time.Now().Format(time.RFC3339),
	}
	if err := pub.Publish(ctx, stream.TopicStatus, evt); err != nil {
		t.Fatalf("publish: %v", err)
	}

	result := <-msgs
	if result.Err != nil {
		t.Fatalf("receive: %v", result.Err)
	}
	if result.Event.NotificationID != evt.NotificationID {
		t.Errorf("got id %s, want %s", result.Event.NotificationID, evt.NotificationID)
	}
	result.Msg.Ack()
}
