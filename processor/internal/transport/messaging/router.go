package messaging

import (
	"context"

	"github.com/ThreeDotsLabs/watermill/message"

	apipub "github.com/barkin94/insider-notification/api/public"
	"github.com/barkin94/insider-notification/processor/internal/delivery"
	stream "github.com/barkin94/insider-notification/shared/messaging"
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
	highMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, string(apipub.TopicHigh), serviceName)
	normalMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, string(apipub.TopicNormal), serviceName)
	lowMsgs := stream.Subscribe[apipub.NotificationReadyEvent](ctx, sub, string(apipub.TopicLow), serviceName)

	return delivery.NewPriorityRouter([]delivery.WeightedSource[stream.Result[apipub.NotificationReadyEvent]]{
		{Ch: highMsgs, Weight: highWeight},
		{Ch: normalMsgs, Weight: normalWeight},
		{Ch: lowMsgs, Weight: lowWeight},
	})
}
