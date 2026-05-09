package stream

import "github.com/ThreeDotsLabs/watermill/message"

// StreamCarrier adapts Watermill message.Metadata to propagation.TextMapCarrier
// so W3C trace context can be propagated through Redis Streams.
type StreamCarrier struct {
	metadata message.Metadata
}

func NewStreamCarrier(metadata message.Metadata) *StreamCarrier {
	return &StreamCarrier{metadata: metadata}
}

func (c *StreamCarrier) Get(key string) string {
	return c.metadata.Get(key)
}

func (c *StreamCarrier) Set(key, value string) {
	c.metadata.Set(key, value)
}

func (c *StreamCarrier) Keys() []string {
	keys := make([]string, 0, len(c.metadata))
	for k := range c.metadata {
		keys = append(keys, k)
	}
	return keys
}
