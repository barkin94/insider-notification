package stream

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

type Publisher struct {
	pub message.Publisher
}

func NewPublisher(pub message.Publisher) *Publisher {
	return &Publisher{pub: pub}
}

func (p *Publisher) Publish(ctx context.Context, topic string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	msg := message.NewMessage(watermill.NewUUID(), b)
	return p.pub.Publish(topic, msg)
}
