package messaging

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/shared/stream"
)

// NewNotificationRouter subscribes to the three priority topics and wires them
// into a weighted PriorityRouter. Caller retains ownership of sub and must
// close it after the router is no longer in use.
func NewNotificationRouter(
	ctx context.Context,
	sub message.Subscriber,
	serviceName string,
	highWeight, normalWeight, lowWeight int,
) (*delivery.PriorityRouter[stream.Result[stream.NotificationReadyEvent]], error) {
	highMsgs, err := stream.Subscribe[stream.NotificationReadyEvent](ctx, sub, stream.TopicHigh, serviceName)
	if err != nil {
		return nil, fmt.Errorf("subscribe high: %w", err)
	}
	normalMsgs, err := stream.Subscribe[stream.NotificationReadyEvent](ctx, sub, stream.TopicNormal, serviceName)
	if err != nil {
		return nil, fmt.Errorf("subscribe normal: %w", err)
	}
	lowMsgs, err := stream.Subscribe[stream.NotificationReadyEvent](ctx, sub, stream.TopicLow, serviceName)
	if err != nil {
		return nil, fmt.Errorf("subscribe low: %w", err)
	}

	return delivery.NewPriorityRouter([]delivery.WeightedSource[stream.Result[stream.NotificationReadyEvent]]{
		{Ch: highMsgs, Weight: highWeight},
		{Ch: normalMsgs, Weight: normalWeight},
		{Ch: lowMsgs, Weight: lowWeight},
	}), nil
}
