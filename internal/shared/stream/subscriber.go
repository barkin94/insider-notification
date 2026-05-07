package stream

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
)

// Subscribe returns a channel of decoded NotificationCreatedEvents for the
// given topic (TopicHigh, TopicNormal, or TopicLow). The caller is responsible
// for calling msg.Ack() or msg.Nack() on each received message.
func Subscribe[T any](ctx context.Context, sub message.Subscriber, topic string) (<-chan Result[T], error) {
	msgs, err := sub.Subscribe(ctx, topic)
	if err != nil {
		return nil, fmt.Errorf("subscribe to %s: %w", topic, err)
	}

	out := make(chan Result[T])
	go func() {
		defer close(out)
		for msg := range msgs {
			var e T
			if err := json.Unmarshal(msg.Payload, &e); err != nil {
				msg.Nack()
				select {
				case out <- Result[T]{Err: fmt.Errorf("unmarshal: %w", err)}:
				case <-ctx.Done():
					return
				}
				continue
			}
			select {
			case out <- Result[T]{Event: e, Msg: msg}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// Result carries a decoded event and its underlying watermill message.
// Call Msg.Ack() after processing or Msg.Nack() to requeue.
type Result[T any] struct {
	Event T
	Msg   *message.Message
	Err   error
}
