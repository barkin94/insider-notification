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

	"github.com/barkin/insider-notification/shared/messaging"
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
		{"high", messaging.TopicHigh},
		{"normal", messaging.TopicNormal},
		{"low", messaging.TopicLow},
	}

	for _, tc := range cases {
		pub := newPublisher(t)
		sub := newSubscriber(t, "test-cg-"+tc.priority)

		msgs := messaging.Subscribe[messaging.NotificationReadyEvent](ctx, sub, tc.topic, "test")

		evt := messaging.NotificationReadyEvent{
			NotificationID: "id-" + tc.priority,
			Channel:        "sms",
			Recipient:      "+1555",
			Content:        "hello",
			Priority:       tc.priority,
			MaxAttempts:    3,
		}
		if err := pub.Publish(ctx, tc.topic, evt); err != nil {
			t.Fatalf("publish: %v", err)
		}

		result := <-msgs
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

	msgs := messaging.Subscribe[messaging.NotificationDeliveryResultEvent](ctx, sub, messaging.TopicStatus, "test")

	evt := messaging.NotificationDeliveryResultEvent{
		NotificationID: "notif-1",
		Status:         "delivered",
		AttemptNumber:  1,
		HTTPStatusCode: 200,
		LatencyMS:      100,
	}
	if err := pub.Publish(ctx, messaging.TopicStatus, evt); err != nil {
		t.Fatalf("publish: %v", err)
	}

	result := <-msgs
	if result.Event.NotificationID != evt.NotificationID {
		t.Errorf("got id %s, want %s", result.Event.NotificationID, evt.NotificationID)
	}
	result.Msg.Ack()
}
