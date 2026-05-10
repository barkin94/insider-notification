package priorityrouter

import (
	"context"
	"sync"
	"time"

	"github.com/barkin/insider-notification/shared/stream"
)

const idleTimeout = time.Second

// PriorityRouter schedules work across three priority channels using weighted round-robin.
type PriorityRouter struct {
	high, normal, low <-chan stream.Result[stream.NotificationCreatedEvent]
	slots             []int // 0=high, 1=normal, 2=low; pre-expanded from weights
	mu                sync.Mutex
	cursor            int
}

// NewPriorityRouter constructs a router with the given weights and channels.
// Panics if any weight < 1.
func NewPriorityRouter(
	_ context.Context,
	high, normal, low <-chan stream.Result[stream.NotificationCreatedEvent],
	weights [3]int,
) *PriorityRouter {
	for _, w := range weights {
		if w < 1 {
			panic("priority router: all weights must be >= 1")
		}
	}
	slots := make([]int, 0, weights[0]+weights[1]+weights[2])
	for i, w := range weights {
		for range w {
			slots = append(slots, i)
		}
	}
	return &PriorityRouter{
		high:   high,
		normal: normal,
		low:    low,
		slots:  slots,
	}
}

// Next returns the next message to process, respecting the weight schedule.
// Returns (result, true) when a message is available.
// Returns (zero, false) when ctx is cancelled or the 1s idle timeout fires.
func (r *PriorityRouter) Next(ctx context.Context) (stream.Result[stream.NotificationCreatedEvent], bool) {
	r.mu.Lock()
	slot := r.slots[r.cursor%len(r.slots)]
	r.cursor++
	r.mu.Unlock()

	ch := r.chanFor(slot)

	// step 2: non-blocking receive from scheduled slot
	select {
	case msg, ok := <-ch:
		if ok {
			return msg, true
		}
	default:
	}

	// step 3: cascade fallback (priority order, non-blocking)
	for _, fb := range []<-chan stream.Result[stream.NotificationCreatedEvent]{r.high, r.normal, r.low} {
		select {
		case msg, ok := <-fb:
			if ok {
				return msg, true
			}
		default:
		}
	}

	// step 4: all channels empty — block with 1s timeout
	select {
	case msg, ok := <-r.high:
		if ok {
			return msg, true
		}
	case msg, ok := <-r.normal:
		if ok {
			return msg, true
		}
	case msg, ok := <-r.low:
		if ok {
			return msg, true
		}
	case <-ctx.Done():
		var zero stream.Result[stream.NotificationCreatedEvent]
		return zero, false
	case <-time.After(idleTimeout):
		var zero stream.Result[stream.NotificationCreatedEvent]
		return zero, false
	}

	var zero stream.Result[stream.NotificationCreatedEvent]
	return zero, false
}

func (r *PriorityRouter) chanFor(slot int) <-chan stream.Result[stream.NotificationCreatedEvent] {
	switch slot {
	case 0:
		return r.high
	case 1:
		return r.normal
	default:
		return r.low
	}
}
