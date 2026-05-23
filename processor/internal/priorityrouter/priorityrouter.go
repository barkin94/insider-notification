package priorityrouter

import (
	"context"
	"sync"
	"time"
)

const idleTimeout = time.Second

// WeightedSource pairs a receive-only channel with its scheduling weight.
// Higher weight means proportionally more slots in the round-robin schedule.
// Weight must be >= 1.
type WeightedSource[T any] struct {
	Ch     <-chan T
	Weight int
}

// PriorityRouter schedules work across an ordered set of channels using
// weighted round-robin. Index 0 is highest priority.
type PriorityRouter[T any] struct {
	sources []<-chan T
	slots   []int // pre-expanded weights; each entry is an index into sources
	mu      sync.Mutex
	cursor  int
}

// NewPriorityRouter constructs a router from an ordered slice of weighted sources.
// Sources are in descending priority order (index 0 = highest).
// Panics if sources is empty or any weight < 1.
func NewPriorityRouter[T any](sources []WeightedSource[T]) *PriorityRouter[T] {
	if len(sources) == 0 {
		panic("priority router: at least one source required")
	}
	totalSlots := 0
	for _, s := range sources {
		if s.Weight < 1 {
			panic("priority router: all weights must be >= 1")
		}
		totalSlots += s.Weight
	}

	slots := make([]int, 0, totalSlots)
	for i, s := range sources {
		for range s.Weight {
			slots = append(slots, i)
		}
	}

	chs := make([]<-chan T, len(sources))
	for i, s := range sources {
		chs[i] = s.Ch
	}

	return &PriorityRouter[T]{
		sources: chs,
		slots:   slots,
	}
}

// Next returns the next value to process, respecting the weight schedule.
// Returns (value, true) when a value is available.
// Returns (zero, false) when ctx is cancelled or the idle timeout fires.
func (r *PriorityRouter[T]) Next(ctx context.Context) (T, bool) {
	r.mu.Lock()
	slot := r.slots[r.cursor%len(r.slots)]
	r.cursor++
	r.mu.Unlock()

	// Step 1: non-blocking receive from the scheduled slot.
	select {
	case v, ok := <-r.sources[slot]:
		if ok {
			return v, true
		}
	default:
	}

	// Step 2: cascade fallback — non-blocking scan in priority order.
	for _, ch := range r.sources {
		select {
		case v, ok := <-ch:
			if ok {
				return v, true
			}
		default:
		}
	}

	// Step 3: all channels empty — block on the highest-priority channel
	// with a 1s idle timeout, then return so the caller can retry.
	// We always block on sources[0] (highest priority) to avoid starvation;
	// the weighted loop above already drained lower-priority channels.
	select {
	case v, ok := <-r.sources[0]:
		if ok {
			return v, true
		}
	case <-ctx.Done():
		var zero T
		return zero, false
	case <-time.After(idleTimeout):
		var zero T
		return zero, false
	}

	var zero T
	return zero, false
}
