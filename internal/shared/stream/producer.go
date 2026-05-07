package stream

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const (
	streamHigh   = "notify:stream:high"
	streamNormal = "notify:stream:normal"
	streamLow    = "notify:stream:low"
	streamStatus = "notify:stream:status"
)

type Producer interface {
	Publish(ctx context.Context, priority string, msg PriorityMessage) error
	PublishStatus(ctx context.Context, msg StatusMessage) error
}

type redisProducer struct{ client *redis.Client }

func NewProducer(client *redis.Client) Producer {
	return &redisProducer{client: client}
}

func priorityStream(priority string) (string, error) {
	switch priority {
	case "high":
		return streamHigh, nil
	case "normal":
		return streamNormal, nil
	case "low":
		return streamLow, nil
	default:
		return "", fmt.Errorf("unknown priority: %s", priority)
	}
}

func (p *redisProducer) Publish(ctx context.Context, priority string, msg PriorityMessage) error {
	stream, err := priorityStream(priority)
	if err != nil {
		return err
	}
	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{
			"notification_id": msg.NotificationID,
			"channel":         msg.Channel,
			"recipient":       msg.Recipient,
			"content":         msg.Content,
			"priority":        msg.Priority,
			"attempt_number":  strconv.Itoa(msg.AttemptNumber),
			"max_attempts":    strconv.Itoa(msg.MaxAttempts),
			"deliver_after":   msg.DeliverAfter,
			"metadata":        msg.Metadata,
		},
	}).Err()
}

func (p *redisProducer) PublishStatus(ctx context.Context, msg StatusMessage) error {
	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamStatus,
		Values: map[string]any{
			"notification_id":     msg.NotificationID,
			"status":              msg.Status,
			"attempt_number":      strconv.Itoa(msg.AttemptNumber),
			"http_status_code":    strconv.Itoa(msg.HTTPStatusCode),
			"error_message":       msg.ErrorMessage,
			"provider_message_id": msg.ProviderMessageID,
			"latency_ms":          strconv.Itoa(msg.LatencyMS),
			"updated_at":          msg.UpdatedAt,
		},
	}).Err()
}
