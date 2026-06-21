package public

type Channel string
type Priority string
type Status string

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
)
