package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel"
)

// Publisher is the port for publishing events to a message stream.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

type redisPublisher struct {
	pub message.Publisher
}

func NewPublisher(pub message.Publisher) Publisher {
	return &redisPublisher{pub: pub}
}

func (p *redisPublisher) Publish(ctx context.Context, topic string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	msg := message.NewMessage(watermill.NewUUID(), b)
	otel.GetTextMapPropagator().Inject(ctx, NewStreamCarrier(msg.Metadata))
	if err := p.pub.Publish(topic, msg); err != nil {
		slog.ErrorContext(ctx, "publish failed", "topic", topic, "error", err)
		return err
	}
	slog.InfoContext(ctx, "publish success", "topic", topic, "payload", payload)
	return nil
}
