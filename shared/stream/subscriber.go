package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Subscribe returns a channel of decoded NotificationCreatedEvents for the
// given topic (TopicHigh, TopicNormal, or TopicLow). The caller is responsible
// for calling msg.Ack() or msg.Nack() on each received message.
func Subscribe[T any](ctx context.Context, sub message.Subscriber, topic, tracerName string) (<-chan Result[T], error) {
	msgs, err := sub.Subscribe(ctx, topic)
	if err != nil {
		return nil, fmt.Errorf("subscribe to %s: %w", topic, err)
	}

	out := make(chan Result[T])
	go func() {
		defer close(out)
		for msg := range msgs {
			// Extract the W3C trace context the publisher injected into metadata.
			msgCtx := otel.GetTextMapPropagator().Extract(ctx, NewStreamCarrier(msg.Metadata))
			// Start a consumer span that continues the distributed trace.
			msgCtx, span := otel.Tracer(tracerName).Start(
				msgCtx,
				fmt.Sprintf("consume %s", topic),
				trace.WithSpanKind(trace.SpanKindConsumer),
			)

			span.SetAttributes(
				attribute.String("messaging.src", topic),
			)
			slog.InfoContext(msgCtx, "message received", "topic", topic)
			var e T
			if err := json.Unmarshal(msg.Payload, &e); err != nil {
				msg.Nack()
				select {
				case out <- Result[T]{Ctx: msgCtx, Err: fmt.Errorf("unmarshal: %w", err)}:
				case <-ctx.Done():
					return
				}
				continue
			}
			select {
			case out <- Result[T]{Ctx: msgCtx, Event: e, Msg: msg}:
			case <-ctx.Done():
				span.End()
				return
			}
		}
	}()
	return out, nil
}

// Result carries a decoded event, its underlying watermill message, and a
// context that has the publisher's trace context extracted from the message
// metadata. Use Ctx (not the caller's ctx) to continue the distributed trace.
type Result[T any] struct {
	Ctx   context.Context
	Event T
	Msg   *message.Message
	Err   error
}
