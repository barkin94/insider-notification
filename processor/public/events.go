package public

// NotificationDeliveryResultEvent is published to TopicStatus by the processor
// after each delivery attempt. Trace context travels in message.Metadata.
type NotificationDeliveryResultEvent struct {
	NotificationID    string
	Status            string
	AttemptNumber     int
	HTTPStatusCode    int
	ErrorMessage      string
	ProviderMessageID string
	LatencyMS         int
}
