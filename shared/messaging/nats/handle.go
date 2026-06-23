package nats

import (
	"fmt"

	natsio "github.com/nats-io/nats.go"
)

// Handle wraps a NATS connection and its JetStream context.
type Handle struct {
	conn *natsio.Conn
	js   natsio.JetStreamContext
}

// NewHandle connects to NATS and opens JetStream. Panics on failure.
func NewHandle(url string) *Handle {
	nc, err := natsio.Connect(url)
	if err != nil {
		panic("nats connect: " + err.Error())
	}
	js, err := nc.JetStream()
	if err != nil {
		panic("nats jetstream: " + err.Error())
	}
	return &Handle{conn: nc, js: js}
}

func (h *Handle) Close() { _ = h.conn.Drain() }

// EnsureStream creates or updates a WorkQueuePolicy stream with FileStorage.
// Idempotent — safe to call on every startup.
func EnsureStream(h *Handle, name string, subjects []string) error {
	cfg := &natsio.StreamConfig{
		Name:      name,
		Subjects:  subjects,
		Retention: natsio.WorkQueuePolicy,
		Storage:   natsio.FileStorage,
	}
	if _, err := h.js.StreamInfo(name); err == natsio.ErrStreamNotFound {
		_, err = h.js.AddStream(cfg)
		return err
	} else if err != nil {
		return fmt.Errorf("nats stream info %s: %w", name, err)
	}
	_, err := h.js.UpdateStream(cfg)
	return err
}
