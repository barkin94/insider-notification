package stream

import (
	"fmt"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

// NewRedisPublisher constructs a Publisher backed by a Redis Stream.
func NewRedisPublisher(client *redis.Client) (*Publisher, error) {
	logger := NewSlogAdapter(slog.Default())
	wPub, err := redisstream.NewPublisher(redisstream.PublisherConfig{Client: client}, logger)
	if err != nil {
		return nil, fmt.Errorf("redis stream publisher: %w", err)
	}
	return NewPublisher(wPub), nil
}

// NewRedisSubscriber constructs a Watermill Subscriber backed by a Redis Stream consumer group.
func NewRedisSubscriber(client *redis.Client, consumerGroup string) (message.Subscriber, error) {
	logger := NewSlogAdapter(slog.Default())
	sub, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        client,
		ConsumerGroup: consumerGroup,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("redis stream subscriber: %w", err)
	}
	return sub, nil
}
