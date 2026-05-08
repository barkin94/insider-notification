package stream

// NotificationCreatedEvent is published to a priority stream when the API
// creates a notification. Trace context travels in message.Metadata.
type NotificationCreatedEvent struct {
	NotificationID string
	Channel        string
	Recipient      string
	Content        string
	Priority       string
	AttemptNumber  int
	MaxAttempts    int
	DeliverAfter   string // RFC3339 or empty
	Metadata       string // JSON string, "{}" if absent
}

// NotificationCancelledEvent is published to the cancellation stream by the API
// when a notification is cancelled. The Processor consumes this to skip in-flight delivery.
type NotificationCancelledEvent struct {
	NotificationID string
}

// NotificationDeliveryResultEvent is published to the status stream by the
// Processor after each delivery attempt. Trace context travels in message.Metadata.
type NotificationDeliveryResultEvent struct {
	NotificationID    string
	Status            string
	AttemptNumber     int
	HTTPStatusCode    int
	ErrorMessage      string
	ProviderMessageID string
	LatencyMS         int
	UpdatedAt         string // RFC3339
}
