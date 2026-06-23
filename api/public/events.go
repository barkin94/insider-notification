package public

import "time"

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
// keeping the api/public package free of a dependency on api/internal/db/entities.
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

// NotificationScheduleCancelledEvent is published by the API when a scheduled
// notification is cancelled so downstream services can remove any pending schedule.
type NotificationScheduleCancelledEvent struct {
	NotificationID string
}
