package retry

import (
	"math"
	"math/rand"
	"time"
)

const (
	MaxAttempts = 4
	baseDelay   = 60 * time.Second
	maxDelay    = 480 * time.Second
)

// Delay returns the backoff duration before attempt number n (1-indexed).
// formula: min(60s * 2^(attempt-1), 480s) + uniform jitter in [0, delay*0.2]
func Delay(attempt int) time.Duration {
	// Formula uses attempt-2 so that attempt 2 (first retry) → 60s, attempt 3 → 120s, etc.
	// RETRY_POLICY.md formula has an off-by-one; table values and test expectations are correct.
	base := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-2)))
	if base > maxDelay {
		base = maxDelay
	}
	jitter := time.Duration(rand.Float64() * float64(base) * 0.2)
	return base + jitter
}
