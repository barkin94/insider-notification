package public

var TopicByPriority = map[Priority]Topic{
	PriorityHigh:   TopicHigh,
	PriorityNormal: TopicNormal,
	PriorityLow:    TopicLow,
}
