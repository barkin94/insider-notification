package db

import "time"

type DeliveryAttempt struct {
	NotificationID string
	AttemptNumber  int
	Priority       string
	RetryAfter     *time.Time
	Channel        string
	Recipient      string
	Content        string
	MaxAttempts    int
}
