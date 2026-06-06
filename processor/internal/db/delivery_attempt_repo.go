package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// DeliveryAttemptRepository is the port for delivery attempt persistence and retry scheduling.
type DeliveryAttemptRepository interface {
	// SavePayload writes the notification payload on first encounter; idempotent on retries.
	// Must be called before any Delay or Create operation so the retry dispatcher can reconstruct the event.
	SavePayload(ctx context.Context, a *DeliveryAttempt) error
	// Create records attempt state (attempt_number + retry_after) and adds to the retry ZSET.
	Create(ctx context.Context, a *DeliveryAttempt) error
	// Delay updates only the retry_after timestamp and the ZSET score.
	// It never touches the notification payload or attempt_number.
	Delay(ctx context.Context, notifID string, retryAfter time.Time) error
	GetAttemptNumber(ctx context.Context, id string) (int, error)
	Delete(ctx context.Context, id string) error
	// GetDue returns attempts whose retry_after is at or before now, up to limit entries.
	GetDue(ctx context.Context, now time.Time, limit int) ([]*DeliveryAttempt, error)
	// RemoveDue removes the ZSET entry for id without deleting the attempt state,
	// so the worker can still read attempt_number after a retry is republished.
	RemoveDue(ctx context.Context, id string) error
}

type redisDeliveryAttemptRepo struct{ client *redis.Client }

var _ DeliveryAttemptRepository = (*redisDeliveryAttemptRepo)(nil)

func NewDeliveryAttemptRepository(client *redis.Client) DeliveryAttemptRepository {
	return &redisDeliveryAttemptRepo{client: client}
}

// attemptTTL caps how long retry state lingers in Redis.
// It must exceed the longest possible retry window (MaxAttempts × max backoff + rate-limit delays).
const attemptTTL = 24 * time.Hour

func (r *redisDeliveryAttemptRepo) SavePayload(ctx context.Context, a *DeliveryAttempt) error {
	key := notificationPayloadKey(a.NotificationID)
	pipe := r.client.Pipeline()
	pipe.HSetNX(ctx, key, "notification_id", a.NotificationID)
	pipe.HSetNX(ctx, key, "channel", a.Channel)
	pipe.HSetNX(ctx, key, "recipient", a.Recipient)
	pipe.HSetNX(ctx, key, "content", a.Content)
	pipe.HSetNX(ctx, key, "priority", a.Priority)
	pipe.HSetNX(ctx, key, "max_attempts", a.MaxAttempts)
	pipe.Expire(ctx, key, attemptTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis ensure notification payload: %w", err)
	}
	return nil
}

func (r *redisDeliveryAttemptRepo) Create(ctx context.Context, a *DeliveryAttempt) error {
	key := deliveryAttemptKey(a.NotificationID)
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, map[string]any{
		"attempt_number": a.AttemptNumber,
		"retry_after":    formatTime(a.RetryAfter),
	})
	pipe.Expire(ctx, key, attemptTTL)
	if a.RetryAfter != nil {
		pipe.ZAdd(ctx, deliveryRetryDueKey, redis.Z{
			Score:  float64(a.RetryAfter.UnixMilli()),
			Member: a.NotificationID,
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis create delivery attempt: %w", err)
	}
	return nil
}

func (r *redisDeliveryAttemptRepo) Delay(ctx context.Context, notifID string, retryAfter time.Time) error {
	key := deliveryAttemptKey(notifID)
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, "retry_after", retryAfter.UTC().Format(time.RFC3339Nano))
	pipe.HSetNX(ctx, key, "attempt_number", 0)
	pipe.Expire(ctx, key, attemptTTL)
	pipe.ZAdd(ctx, deliveryRetryDueKey, redis.Z{
		Score:  float64(retryAfter.UnixMilli()),
		Member: notifID,
	})
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis delay delivery attempt: %w", err)
	}
	return nil
}

// GetAttemptNumber returns the attempt_number stored for the notification,
// or 0 if no attempt has been recorded yet.
func (r *redisDeliveryAttemptRepo) GetAttemptNumber(ctx context.Context, id string) (int, error) {
	raw, err := r.client.HGet(ctx, deliveryAttemptKey(id), "attempt_number").Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("redis hget delivery attempt: %w", err)
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse attempt_number %q: %w", raw, err)
	}
	return n, nil
}

func (r *redisDeliveryAttemptRepo) Delete(ctx context.Context, id string) error {
	pipe := r.client.Pipeline()
	pipe.Del(ctx, deliveryAttemptKey(id))
	pipe.Del(ctx, notificationPayloadKey(id))
	pipe.ZRem(ctx, deliveryRetryDueKey, id)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis del delivery attempt: %w", err)
	}
	return nil
}

func (r *redisDeliveryAttemptRepo) GetDue(ctx context.Context, now time.Time, limit int) ([]*DeliveryAttempt, error) {
	if limit < 1 {
		limit = 1
	}
	ids, err := r.client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     deliveryRetryDueKey,
		Start:   "-inf",
		Stop:    strconv.FormatInt(now.UTC().UnixMilli(), 10),
		ByScore: true,
		Count:   int64(limit),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("redis zrange delivery retry: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Batch-fetch both hashes for all due IDs in one round-trip.
	pipe := r.client.Pipeline()
	stateCmds := make([]*redis.MapStringStringCmd, len(ids))
	payloadCmds := make([]*redis.MapStringStringCmd, len(ids))
	for i, id := range ids {
		stateCmds[i] = pipe.HGetAll(ctx, deliveryAttemptKey(id))
		payloadCmds[i] = pipe.HGetAll(ctx, notificationPayloadKey(id))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("redis pipeline get due attempts: %w", err)
	}

	attempts := make([]*DeliveryAttempt, 0, len(ids))
	for i, id := range ids {
		state, _ := stateCmds[i].Result()
		payload, _ := payloadCmds[i].Result()
		if len(state) == 0 || len(payload) == 0 {
			_ = r.RemoveDue(ctx, id)
			continue
		}
		a, err := parseAttempt(id, state, payload)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, nil
}

func (r *redisDeliveryAttemptRepo) RemoveDue(ctx context.Context, id string) error {
	if err := r.client.ZRem(ctx, deliveryRetryDueKey, id).Err(); err != nil {
		return fmt.Errorf("redis zrem delivery retry: %w", err)
	}
	return nil
}

func parseAttempt(notifID string, state, payload map[string]string) (*DeliveryAttempt, error) {
	attemptNumber, err := parseIntField(state, "attempt_number")
	if err != nil {
		return nil, err
	}
	maxAttempts, err := parseIntField(payload, "max_attempts")
	if err != nil {
		return nil, err
	}
	retryAfter, err := parseOptionalTime(state["retry_after"])
	if err != nil {
		return nil, err
	}
	return &DeliveryAttempt{
		NotificationID: notifID,
		AttemptNumber:  attemptNumber,
		Priority:       payload["priority"],
		RetryAfter:     retryAfter,
		Channel:        payload["channel"],
		Recipient:      payload["recipient"],
		Content:        payload["content"],
		MaxAttempts:    maxAttempts,
	}, nil
}

func parseIntField(values map[string]string, field string) (int, error) {
	n, err := strconv.Atoi(values[field])
	if err != nil {
		return 0, fmt.Errorf("parse %s %q: %w", field, values[field], err)
	}
	return n, nil
}

func parseOptionalTime(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, fmt.Errorf("parse retry_after %q: %w", raw, err)
	}
	return &t, nil
}

const deliveryRetryDueKey = "processor:delivery_retry:due"

func deliveryAttemptKey(id string) string {
	return "processor:delivery_attempt:{" + id + "}"
}

func notificationPayloadKey(id string) string {
	return "processor:notification_payload:{" + id + "}"
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
