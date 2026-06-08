package stream

import (
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

// NewRedisPublisher constructs a Publisher backed by a Redis Stream. Panics if client is nil.
func NewRedisPublisher(client *redis.Client) Publisher {
	logger := NewSlogAdapter(slog.Default())
	wPub, err := redisstream.NewPublisher(redisstream.PublisherConfig{Client: client}, logger)
	if err != nil {
		panic("redis stream publisher: " + err.Error())
	}
	return NewPublisher(wPub)
}

// NewRedisSubscriber constructs a Watermill Subscriber backed by a Redis Stream consumer group. Panics if client is nil.
func NewRedisSubscriber(client *redis.Client, consumerGroup string) message.Subscriber {
	logger := NewSlogAdapter(slog.Default())
	sub, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:          client,
		ConsumerGroup:   consumerGroup,
		NackResendSleep: 5 * time.Second,
	}, logger)
	if err != nil {
		panic("redis stream subscriber: " + err.Error())
	}
	return sub
}
