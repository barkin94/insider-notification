package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	capacity   = 100
	refillRate = 100 // tokens per second
	burst      = 120
)

// Lua atomic token bucket.
// Hash fields: tokens (float), last_refill (unix seconds float).
var luaScript = redis.NewScript(`
local key        = KEYS[1]
local now        = tonumber(ARGV[1])
local capacity   = tonumber(ARGV[2])
local refill     = tonumber(ARGV[3])
local burst      = tonumber(ARGV[4])

local data       = redis.call("HMGET", key, "tokens", "last_refill")
local tokens     = tonumber(data[1])
local last       = tonumber(data[2])

if tokens == nil then
    tokens = burst
    last   = now
end

local elapsed = now - last
local added   = elapsed * refill
tokens = math.min(burst, tokens + added)

if tokens >= 1 then
    tokens = tokens - 1
    redis.call("HMSET", key, "tokens", tokens, "last_refill", now)
    redis.call("EXPIRE", key, 3600)
    return 1
end

redis.call("HMSET", key, "tokens", tokens, "last_refill", now)
redis.call("EXPIRE", key, 3600)
return 0
`)

// Limiter checks whether a delivery attempt for a given channel is allowed.
type Limiter interface {
	Allow(ctx context.Context, channel string) (bool, error)
}

type redisLimiter struct {
	client *redis.Client
}

func NewLimiter(client *redis.Client) Limiter {
	return &redisLimiter{client: client}
}

func (l *redisLimiter) Allow(ctx context.Context, channel string) (bool, error) {
	key := fmt.Sprintf("ratelimit:{%s}", channel)
	now := float64(time.Now().UnixNano()) / 1e9 // fractional seconds

	result, err := luaScript.Run(ctx, l.client, []string{key},
		now, capacity, refillRate, burst,
	).Int()
	if err != nil {
		return false, fmt.Errorf("rate limiter script: %w", err)
	}
	return result == 1, nil
}
