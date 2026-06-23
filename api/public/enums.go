package public

type Channel string
type Priority string
type Status string
type Topic string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"

	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"

	StatusPending   Status = "pending"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"

	TopicHigh                          Topic = "notify.high"
	TopicNormal                        Topic = "notify.normal"
	TopicLow                           Topic = "notify.low"
	TopicNotificationScheduled         Topic = "notify.scheduled"
	TopicNotificationScheduleCancelled Topic = "notify.schedule.cancelled"
)
