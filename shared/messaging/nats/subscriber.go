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
// message (for Ack/Nak/NakWithDelay), and the 1-indexed attempt number sourced
// from the broker's NumDelivered counter. Call EndSpan after Ack/Nak to close the span.
type Result[T any] struct {
	Ctx           context.Context
	Event         T
	Msg           *natsio.Msg
	AttemptNumber int
}

// EndSpan ends the consumer span that was started when the message was decoded.
// Call this after Ack/Nak so the span covers the full processing lifecycle.
func (r Result[T]) EndSpan() {
	trace.SpanFromContext(r.Ctx).End()
}

// SubscribeBatch is like Subscribe but delivers messages in fetch-sized slices
// rather than individually. Handlers receive the full batch from each Fetch
// call, enabling bulk DB lookups and pipelined publishes without re-batching.
// Panics if the subscription cannot be established.
func SubscribeBatch[T any](
	ctx context.Context,
	h *Handle,
	subject, durableName, tracerName string,
	maxDeliver int,
) <-chan []Result[T] {
	sub, err := h.js.PullSubscribe(subject, durableName,
		natsio.AckExplicit(),
		natsio.MaxDeliver(maxDeliver),
		natsio.AckWait(60*time.Second),
	)
	if err != nil {
		panic("nats pull subscribe " + subject + ": " + err.Error())
	}
	slog.InfoContext(ctx, "nats batch subscription established", "subject", subject, "durable", durableName)

	// Buffer 4 batches so the fetch loop stays ahead of a slow consumer
	// without blocking; deeper buffering would hide backpressure from NATS.
	out := make(chan []Result[T], 4)
	go func() {
		// Drain flushes any pending acks before the subscription is closed.
		defer func() {
			slog.InfoContext(ctx, "nats batch subscription closed", "subject", subject)
			_ = sub.Drain()
			close(out)
		}()
		for ctx.Err() == nil {
			// Fetch up to 32 messages; MaxWait avoids blocking forever so
			// ctx cancellation is checked on every iteration.
			msgs, err := sub.Fetch(32, natsio.MaxWait(2*time.Second))
			if err != nil {
				// ErrTimeout is a normal empty-poll, not a real error.
				if !errors.Is(err, natsio.ErrTimeout) {
					slog.Error("nats fetch", "subject", subject, "error", err)
				}
				continue
			}

			// Decode each message; undecipherable payloads are NAKed inside
			// decode and excluded from the batch so one bad message doesn't
			// block the rest.
			batch := make([]Result[T], 0, len(msgs))
			for _, msg := range msgs {
				r, ok := decode[T](ctx, msg, subject, tracerName)
				if !ok {
					continue
				}
				batch = append(batch, r)
			}
			if len(batch) == 0 {
				continue
			}

			// Send the batch or NAK every message if the context was cancelled
			// while we were waiting; NAKing lets the broker redeliver them to
			// another consumer instead of waiting for the AckWait timeout.
			select {
			case out <- batch:
				slog.DebugContext(ctx, "nats batch dispatched", "subject", subject, "size", len(batch))
			case <-ctx.Done():
				slog.InfoContext(ctx, "nats context cancelled, naking batch", "subject", subject, "size", len(batch))
				for _, r := range batch {
					_ = r.Msg.Nak()
					r.EndSpan()
				}
				return
			}
		}
	}()
	return out
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
	slog.InfoContext(ctx, "nats subscription established", "subject", subject, "durable", durableName)

	// Buffer matches the fetch batch size so the goroutine can hand off a
	// full fetch in one shot without blocking on the first send.
	out := make(chan Result[T], 32)
	go func() {
		// Drain flushes any pending acks before the subscription is closed.
		defer func() {
			slog.InfoContext(ctx, "nats subscription closed", "subject", subject)
			_ = sub.Drain()
			close(out)
		}()
		for ctx.Err() == nil {
			// Fetch up to 32 messages; MaxWait avoids blocking forever so
			// ctx cancellation is checked on every iteration.
			msgs, err := sub.Fetch(32, natsio.MaxWait(2*time.Second))
			if err != nil {
				// ErrTimeout is a normal empty-poll, not a real error.
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

				// NAK and exit if ctx was cancelled while we were blocked
				// sending; lets the broker redeliver to another consumer.
				select {
				case out <- r:
				case <-ctx.Done():
					slog.InfoContext(ctx, "nats context cancelled, naking message", "subject", subject)
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
	// Restore the producer's trace context from the message headers so the
	// consumer span is a child of the publish span in the same trace.
	msgCtx := otel.GetTextMapPropagator().Extract(ctx, headerCarrier{msg.Header})
	msgCtx, span := otel.Tracer(tracerName).Start(msgCtx, "consume "+subject, trace.WithSpanKind(trace.SpanKindConsumer))

	// NumDelivered is 1-indexed: 1 on first delivery, 2 on first retry, etc.
	// Default to 1 if metadata is unavailable (e.g. non-JetStream messages).
	attemptNumber := 1
	if meta, err := msg.Metadata(); err == nil {
		attemptNumber = int(min(meta.NumDelivered, math.MaxInt)) //nolint:gosec
	}

	// Tag the span with standard OTel messaging attributes for observability.
	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.operation.name", "process"),
		attribute.String("messaging.destination.name", subject),
		attribute.Int("messaging.message.delivery_count", attemptNumber),
	)

	// NAK on unmarshal failure so the broker retries up to maxDeliver times;
	// end the span here since the caller won't receive a Result to call EndSpan on.
	var e T
	if err := json.Unmarshal(msg.Data, &e); err != nil {
		slog.ErrorContext(msgCtx, "nats unmarshal, nacking", "subject", subject, "error", err)
		_ = msg.Nak()
		span.End()
		return Result[T]{}, false
	}
	slog.InfoContext(msgCtx, "nats message received", "subject", subject, "attempt_number", attemptNumber)
	return Result[T]{Ctx: msgCtx, Event: e, Msg: msg, AttemptNumber: attemptNumber}, true
}
