package nats

import natsio "github.com/nats-io/nats.go"

// headerCarrier adapts nats.Header to propagation.TextMapCarrier
// so W3C trace context can be propagated through NATS message headers.
type headerCarrier struct{ h natsio.Header }

func (c headerCarrier) Get(key string) string { return c.h.Get(key) }
func (c headerCarrier) Set(key, value string) { c.h.Set(key, value) }
func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c.h))
	for k := range c.h {
		keys = append(keys, k)
	}
	return keys
}
