package messaging

import (
	apipub "github.com/barkin/insider-notification/api/public"
)

var topicByPriority = map[string]string{
	string(apipub.PriorityHigh):   apipub.TopicHigh,
	string(apipub.PriorityNormal): apipub.TopicNormal,
	string(apipub.PriorityLow):    apipub.TopicLow,
}

func topicForPriority(priority string) string {
	topic := topicByPriority[priority]
	if topic != "" {
		return topic
	}
	return topicByPriority[string(apipub.PriorityNormal)]
}
