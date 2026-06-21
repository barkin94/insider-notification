package messaging

import (
	"context"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/barkin/insider-notification/processor/internal/delivery"
	apipub "github.com/barkin/insider-notification/api/public"
	stream "github.com/barkin/insider-notification/shared/messaging"
)

// NewNotificationRouter subscribes to the three priority topics and wires them
// into a weighted PriorityRouter. Caller retains ownership of sub and must
// close it after the router is no longer in use.
func NewNotificationRouter(
	ctx context.Context,
	sub message.Subscriber,
	serviceName string,
	highWeight, normalWeight, lowWeight int,
) *delivery.PriorityRouter[stream.Result[apipub.NotificationReadyEvent]] {
	highMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, apipub.TopicHigh, serviceName)
	normalMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, apipub.TopicNormal, serviceName)
	lowMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, apipub.TopicLow, serviceName)

	return delivery.NewPriorityRouter([]delivery.WeightedSource[stream.Result[apipub.NotificationReadyEvent]]{
		{Ch: highMsgs, Weight: highWeight},
		{Ch: normalMsgs, Weight: normalWeight},
		{Ch: lowMsgs, Weight: lowWeight},
	})
}
