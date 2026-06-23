package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	natsio "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"

	messaging "github.com/barkin94/insider-notification/shared/messaging"
)

// NewPublisher constructs a Publisher that sends JSON-encoded events to NATS
// subjects via JetStream, waiting for server-side persistence acknowledgement.
func NewPublisher(h *Handle) messaging.Publisher {
	return &publisher{h: h}
}

type publisher struct{ h *Handle }

func (p *publisher) Publish(ctx context.Context, subject string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("nats marshal payload: %w", err)
	}
	msg := &natsio.Msg{
		Subject: subject,
		Data:    b,
		Header:  make(natsio.Header),
	}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier{msg.Header})
	if _, err := p.h.js.PublishMsg(msg); err != nil {
		slog.ErrorContext(ctx, "nats publish failed", "subject", subject, "error", err)
		return err
	}
	return nil
}

// NewRoutingPublisher returns a Publisher that delegates to per-topic publishers,
// using fallback for any topic not listed in routes.
func NewRoutingPublisher(routes map[string]messaging.Publisher, fallback messaging.Publisher) messaging.Publisher {
	return &routingPublisher{routes: routes, fallback: fallback}
}

type routingPublisher struct {
	routes   map[string]messaging.Publisher
	fallback messaging.Publisher
}

func (r *routingPublisher) Publish(ctx context.Context, topic string, payload any) error {
	if pub, ok := r.routes[topic]; ok {
		return pub.Publish(ctx, topic, payload)
	}
	return r.fallback.Publish(ctx, topic, payload)
}
