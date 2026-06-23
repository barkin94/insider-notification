package messaging

import "context"

// Publisher is the port for publishing events to a message stream.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

// BatchMessage pairs a per-message context (for trace propagation) with its payload.
type BatchMessage struct {
	Ctx     context.Context
	Payload any
}

// BatchPublisher extends Publisher with a pipelined multi-message publish.
// Each BatchMessage carries its own context so OTel trace headers are injected
// per message rather than shared across the batch. The outer ctx controls
// cancellation of the ack-wait after all messages are sent.
// Collapses N sequential round-trips into one server round-trip.
type BatchPublisher interface {
	Publisher
	PublishBatch(ctx context.Context, topic string, messages []BatchMessage) error
}
