package stream

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	processorGroup = "notify:cg:processor"
	apiGroup       = "notify:cg:api"
)

type Consumer interface {
	ReadPriority(ctx context.Context) (*PriorityMessage, string, error)
	ReadStatus(ctx context.Context) (*StatusMessage, string, error)
	Ack(ctx context.Context, stream, msgID string) error
	ReclaimStale(ctx context.Context, stream string, minIdle time.Duration) error
}

type redisConsumer struct {
	client       *redis.Client
	groupName    string
	consumerName string
}

func NewConsumer(ctx context.Context, client *redis.Client, groupName, consumerName string) (Consumer, error) {
	streams := []string{streamHigh, streamNormal, streamLow, streamStatus}
	for _, s := range streams {
		err := client.XGroupCreateMkStream(ctx, s, groupName, "0").Err()
		if err != nil && !isBusyGroup(err) {
			return nil, err
		}
	}
	return &redisConsumer{client: client, groupName: groupName, consumerName: consumerName}, nil
}

func isBusyGroup(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) >= 9 && msg[:9] == "BUSYGROUP"
}

func (c *redisConsumer) ReadPriority(ctx context.Context) (*PriorityMessage, string, error) {
	priorityStreams := []string{streamHigh, streamNormal, streamLow}

	// Non-blocking sweep: high → normal → low
	for _, s := range priorityStreams {
		msgs, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.groupName,
			Consumer: c.consumerName,
			Streams:  []string{s, ">"},
			Count:    1,
			Block:    -1,
		}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, "", err
		}
		if len(msgs) > 0 && len(msgs[0].Messages) > 0 {
			return parsePriorityMessage(msgs[0].Messages[0])
		}
	}

	// Block on high stream for up to 1 second
	msgs, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.groupName,
		Consumer: c.consumerName,
		Streams:  []string{streamHigh, ">"},
		Count:    1,
		Block:    time.Second,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, "", nil
		}
		return nil, "", err
	}
	if len(msgs) > 0 && len(msgs[0].Messages) > 0 {
		return parsePriorityMessage(msgs[0].Messages[0])
	}
	return nil, "", nil
}

func (c *redisConsumer) ReadStatus(ctx context.Context) (*StatusMessage, string, error) {
	msgs, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.groupName,
		Consumer: c.consumerName,
		Streams:  []string{streamStatus, ">"},
		Count:    1,
		Block:    time.Second,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, "", nil
		}
		return nil, "", err
	}
	if len(msgs) == 0 || len(msgs[0].Messages) == 0 {
		return nil, "", nil
	}
	return parseStatusMessage(msgs[0].Messages[0])
}

func (c *redisConsumer) Ack(ctx context.Context, stream, msgID string) error {
	return c.client.XAck(ctx, stream, c.groupName, msgID).Err()
}

func (c *redisConsumer) ReclaimStale(ctx context.Context, stream string, minIdle time.Duration) error {
	cursor := "0-0"
	for {
		_, next, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   stream,
			Group:    c.groupName,
			Consumer: c.consumerName,
			MinIdle:  minIdle,
			Start:    cursor,
			Count:    100,
		}).Result()
		if err != nil {
			return err
		}
		cursor = next
		if cursor == "0-0" {
			break
		}
	}
	return nil
}

func parsePriorityMessage(m redis.XMessage) (*PriorityMessage, string, error) {
	v := m.Values
	msg := &PriorityMessage{
		NotificationID: strVal(v, "notification_id"),
		Channel:        strVal(v, "channel"),
		Recipient:      strVal(v, "recipient"),
		Content:        strVal(v, "content"),
		Priority:       strVal(v, "priority"),
		AttemptNumber:  intVal(v, "attempt_number"),
		MaxAttempts:    intVal(v, "max_attempts"),
		DeliverAfter:   strVal(v, "deliver_after"),
		Metadata:       strVal(v, "metadata"),
	}
	return msg, m.ID, nil
}

func parseStatusMessage(m redis.XMessage) (*StatusMessage, string, error) {
	v := m.Values
	msg := &StatusMessage{
		NotificationID:    strVal(v, "notification_id"),
		Status:            strVal(v, "status"),
		AttemptNumber:     intVal(v, "attempt_number"),
		HTTPStatusCode:    intVal(v, "http_status_code"),
		ErrorMessage:      strVal(v, "error_message"),
		ProviderMessageID: strVal(v, "provider_message_id"),
		LatencyMS:         intVal(v, "latency_ms"),
		UpdatedAt:         strVal(v, "updated_at"),
	}
	return msg, m.ID, nil
}

func strVal(v map[string]any, key string) string {
	if s, ok := v[key].(string); ok {
		return s
	}
	return ""
}

func intVal(v map[string]any, key string) int {
	s := strVal(v, key)
	n, _ := strconv.Atoi(s)
	return n
}
