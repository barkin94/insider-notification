package public

import "time"

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

// NotificationRetryScheduleEvent is published to TopicRetry by the processor
// whenever a delivery attempt should be retried after a delay.
// ScheduledAt is the absolute UTC time at which the attempt should be retried.
type NotificationRetryScheduleEvent struct {
	NotificationID string
	Channel        string
	Recipient      string
	Content        string
	Priority       string
	MaxAttempts    int
	AttemptNumber  int
	ScheduledAt    time.Time
}
