package messaging

import "time"

// NotificationReadyEvent is published to a priority stream when a notification
// is ready to be delivered immediately. Trace context travels in message.Metadata.
// AttemptNumber is 0 for first-time delivery (API-originated); the retry dispatcher
// sets it from stored state when re-publishing a failed attempt.
type NotificationReadyEvent struct {
	NotificationID string
	Channel        string
	Recipient      string
	Content        string
	Priority       string
	MaxAttempts    int
	AttemptNumber  int
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

// NotificationsScheduledEvent is published to TopicNotificationScheduled by the API
// when one or more notifications are created with a future delivery time (single or batch).
// The scheduler service consumes this and persists each schedule.
type NotificationsScheduledEvent struct {
	Notifications []ScheduledNotificationItem
}

type ScheduledNotificationItem struct {
	NotificationID string
	ScheduledAt    time.Time
}

// ScheduledNotificationDueEvent is published to TopicScheduledNotificationDue
// by the scheduler service when notifications with a past scheduled_at are due.
// The API service consumes this, hydrates full notification details from its DB,
// and publishes NotificationReadyEvent to the processor.
type ScheduledNotificationDueEvent struct {
	NotificationIDs []string
}
