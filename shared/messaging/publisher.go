package messaging

import "context"

// Publisher is the port for publishing events to a message stream.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}
