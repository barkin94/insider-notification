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

// Result carries a decoded event alongside its watermill message and a context
// with the publisher's trace propagated from the message metadata.
type Result[T any] struct {
	Ctx   context.Context
	Event T
	Msg   *message.Message
}

// Subscribe returns a channel of decoded events for the given topic. The caller
// is responsible for calling msg.Ack() or msg.Nack() on each received message.
// Panics if the underlying subscription cannot be established.
func Subscribe[T any](ctx context.Context, sub message.Subscriber, topic, tracerName string) <-chan Result[T] {
	msgs, err := sub.Subscribe(ctx, topic)
	if err != nil {
		panic(fmt.Sprintf("subscribe to %s: %s", topic, err))
	}

	out := make(chan Result[T])

	// handleMessage processes a single message: extracts the publisher's trace,
	// decodes the payload, and forwards the result to out. Returns false when the
	// context is cancelled, signalling the consumer loop to stop.
	handleMessage := func(msg *message.Message) bool {
		// Propagate the publisher's trace context carried in the message metadata.
		msgCtx := otel.GetTextMapPropagator().Extract(ctx, NewStreamCarrier(msg.Metadata))
		msgCtx, span := otel.Tracer(tracerName).Start(msgCtx, "consume "+topic, trace.WithSpanKind(trace.SpanKindConsumer))
		defer span.End()

		span.SetAttributes(attribute.String("messaging.src", topic))
		slog.InfoContext(msgCtx, "message received", "topic", topic)

		// Nack and skip malformed payloads so they don't block the consumer.
		var e T
		if err := json.Unmarshal(msg.Payload, &e); err != nil {
			slog.ErrorContext(msgCtx, "unmarshal failed, skipping message", "topic", topic, "error", err)
			msg.Nack()
			return true
		}

		// Block until the caller accepts the result or the context is cancelled.
		select {
		case out <- Result[T]{Ctx: msgCtx, Event: e, Msg: msg}:
			return true
		case <-ctx.Done():
			return false
		}
	}

	go func() {
		defer close(out)
		for msg := range msgs {
			if !handleMessage(msg) {
				return
			}
		}
	}()
	return out
}
