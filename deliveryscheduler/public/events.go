package public

// ScheduledNotificationDueEvent is published to TopicScheduledNotificationDue
// by the deliveryscheduler when a notification with a past scheduled_at is due.
// The API service consumes this, hydrates full notification details from its DB,
// and publishes NotificationReadyEvent to the processor.
type ScheduledNotificationDueEvent struct {
	NotificationID string
}
