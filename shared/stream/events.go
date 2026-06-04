package stream

// NotificationReadyEvent is published to a priority stream when a notification
// is ready to be delivered immediately. Trace context travels in message.Metadata.
type NotificationReadyEvent struct {
	NotificationID string
	Channel        string
	Recipient      string
	Content        string
	Priority       string
	MaxAttempts    int
}

// NotificationEntity is the minimal interface NotificationReadyEvent.From requires,
// keeping the stream package free of a dependency on api/internal/db/entities.
type NotificationEntity interface {
	GetID() string
	GetChannel() string
	GetRecipient() string
	GetContent() string
	GetPriority() string
	GetMaxAttempts() int
}

func (NotificationReadyEvent) From(n NotificationEntity) NotificationReadyEvent {
	return NotificationReadyEvent{
		NotificationID: n.GetID(),
		Channel:        n.GetChannel(),
		Recipient:      n.GetRecipient(),
		Content:        n.GetContent(),
		Priority:       n.GetPriority(),
		MaxAttempts:    n.GetMaxAttempts(),
	}
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
