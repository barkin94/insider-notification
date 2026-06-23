package nats

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"time"

	natsio "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Result carries a decoded event, its trace-propagated context, the raw NATS
// message (for Ack/Nak/NakWithDelay), the pre-computed delivery count, and
// the active consumer span. Call EndSpan after Ack/Nak to close the span.
type Result[T any] struct {
	Ctx           context.Context
	Event         T
	Msg           *natsio.Msg
	DeliveryCount int
}

// EndSpan ends the consumer span that was started when the message was decoded.
// Call this after Ack/Nak so the span covers the full processing lifecycle.
func (r Result[T]) EndSpan() {
	trace.SpanFromContext(r.Ctx).End()
}

// Subscribe creates a pull-based JetStream consumer for subject.
// Pull semantics give natural backpressure: we only fetch the next batch
// when the worker pool is ready, preventing the redelivery storms that
// push consumers cause when the internal channel fills up under load.
// Panics if the subscription cannot be established.
func Subscribe[T any](
	ctx context.Context,
	h *Handle,
	subject, durableName, tracerName string,
	maxDeliver int,
) <-chan Result[T] {
	sub, err := h.js.PullSubscribe(subject, durableName,
		natsio.AckExplicit(),
		natsio.MaxDeliver(maxDeliver),
		natsio.AckWait(60*time.Second),
	)
	if err != nil {
		panic("nats pull subscribe " + subject + ": " + err.Error())
	}

	out := make(chan Result[T], 32)
	go func() {
		defer func() { _ = sub.Drain(); close(out) }()
		for ctx.Err() == nil {
			msgs, err := sub.Fetch(32, natsio.MaxWait(2*time.Second))
			if err != nil {
				if !errors.Is(err, natsio.ErrTimeout) {
					slog.Error("nats fetch", "subject", subject, "error", err)
				}
				continue
			}
			for _, msg := range msgs {
				r, ok := decode[T](ctx, msg, subject, tracerName)
				if !ok {
					continue
				}
				select {
				case out <- r:
				case <-ctx.Done():
					_ = msg.Nak()
					r.EndSpan()
					return
				}
			}
		}
	}()
	return out
}

func decode[T any](ctx context.Context, msg *natsio.Msg, subject, tracerName string) (Result[T], bool) {
	msgCtx := otel.GetTextMapPropagator().Extract(ctx, headerCarrier{msg.Header})
	msgCtx, span := otel.Tracer(tracerName).Start(msgCtx, "consume "+subject, trace.WithSpanKind(trace.SpanKindConsumer))

	deliveryCount := 1
	if meta, err := msg.Metadata(); err == nil {
		deliveryCount = int(min(meta.NumDelivered, math.MaxInt)) //nolint:gosec // delivery counts never exceed MaxInt in practice
	}

	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.operation.name", "process"),
		attribute.String("messaging.destination.name", subject),
		attribute.Int("messaging.message.delivery_count", deliveryCount),
	)

	var e T
	if err := json.Unmarshal(msg.Data, &e); err != nil {
		slog.ErrorContext(msgCtx, "nats unmarshal, nacking", "subject", subject, "error", err)
		_ = msg.Nak()
		span.End()
		return Result[T]{}, false
	}
	return Result[T]{Ctx: msgCtx, Event: e, Msg: msg, DeliveryCount: deliveryCount}, true
}
