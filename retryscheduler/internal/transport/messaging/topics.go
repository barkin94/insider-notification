package messaging

import (
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
)

var topicByPriority = map[string]string{
	string(model.PriorityHigh):   stream.TopicHigh,
	string(model.PriorityNormal): stream.TopicNormal,
	string(model.PriorityLow):    stream.TopicLow,
}

func topicForPriority(priority string) string {
	topic := topicByPriority[priority]
	if topic != "" {
		return topic
	}
	return topicByPriority[string(model.PriorityNormal)]
}
