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

// Subscribe returns a channel of decoded events for the given topic. The caller
// is responsible for calling msg.Ack() or msg.Nack() on each received message.
// Panics if the underlying subscription cannot be established.
func Subscribe[T any](ctx context.Context, sub message.Subscriber, topic, tracerName string) <-chan Result[T] {
	msgs, err := sub.Subscribe(ctx, topic)
	if err != nil {
		panic(fmt.Sprintf("subscribe to %s: %s", topic, err))
	}

	out := make(chan Result[T])
	go func() {
		defer close(out)
		for msg := range msgs {
			if !consumeMsg(ctx, msg, topic, tracerName, out) {
				return
			}
		}
	}()
	return out
}

// consumeMsg handles a single message: extracts trace context, opens a consumer
// span, decodes the payload, and forwards the result. Returns false when ctx is
// cancelled and the caller should stop the loop.
func consumeMsg[T any](ctx context.Context, msg *message.Message, topic, tracerName string, out chan<- Result[T]) bool {
	msgCtx := otel.GetTextMapPropagator().Extract(ctx, NewStreamCarrier(msg.Metadata))
	msgCtx, span := otel.Tracer(tracerName).Start(
		msgCtx,
		fmt.Sprintf("consume %s", topic),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	span.SetAttributes(attribute.String("messaging.src", topic))
	slog.InfoContext(msgCtx, "message received", "topic", topic)

	var e T
	if err := json.Unmarshal(msg.Payload, &e); err != nil {
		slog.ErrorContext(msgCtx, "unmarshal failed, skipping message", "topic", topic, "error", err)
		msg.Nack()
		return true
	}

	select {
	case out <- Result[T]{Ctx: msgCtx, Event: e, Msg: msg}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Result carries a decoded event, its underlying watermill message, and a
// context that has the publisher's trace context extracted from the message
// metadata. Use Ctx (not the caller's ctx) to continue the distributed trace.
type Result[T any] struct {
	Ctx   context.Context
	Event T
	Msg   *message.Message
}
