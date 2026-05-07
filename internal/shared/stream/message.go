package stream

type PriorityMessage struct {
	NotificationID string
	Channel        string
	Recipient      string
	Content        string
	Priority       string
	AttemptNumber  int
	MaxAttempts    int
	DeliverAfter   string
	Metadata       string
}

type StatusMessage struct {
	NotificationID    string
	Status            string
	AttemptNumber     int
	HTTPStatusCode    int
	ErrorMessage      string
	ProviderMessageID string
	LatencyMS         int
	UpdatedAt         string
}
