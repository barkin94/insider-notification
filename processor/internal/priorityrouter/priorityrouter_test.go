package priorityrouter_test

import (
	"context"
	"testing"
	"time"

	"github.com/barkin/insider-notification/processor/internal/priorityrouter"
	"github.com/barkin/insider-notification/shared/stream"
)

func makeResult(priority string) stream.Result[stream.NotificationCreatedEvent] {
	return stream.Result[stream.NotificationCreatedEvent]{
		Event: stream.NotificationCreatedEvent{Priority: priority},
	}
}

func filledChan(results []stream.Result[stream.NotificationCreatedEvent]) <-chan stream.Result[stream.NotificationCreatedEvent] {
	ch := make(chan stream.Result[stream.NotificationCreatedEvent], len(results))
	for _, r := range results {
		ch <- r
	}
	return ch
}

func emptyChan() <-chan stream.Result[stream.NotificationCreatedEvent] {
	return make(chan stream.Result[stream.NotificationCreatedEvent])
}

func TestWeightedRatio(t *testing.T) {
	const total = 600
	highMsgs := make([]stream.Result[stream.NotificationCreatedEvent], 300)
	normalMsgs := make([]stream.Result[stream.NotificationCreatedEvent], 200)
	lowMsgs := make([]stream.Result[stream.NotificationCreatedEvent], 100)
	for i := range highMsgs {
		highMsgs[i] = makeResult("high")
	}
	for i := range normalMsgs {
		normalMsgs[i] = makeResult("normal")
	}
	for i := range lowMsgs {
		lowMsgs[i] = makeResult("low")
	}

	r := priorityrouter.NewPriorityRouter(
		context.Background(),
		filledChan(highMsgs), filledChan(normalMsgs), filledChan(lowMsgs),
		[3]int{3, 2, 1},
	)

	counts := map[string]int{}
	ctx := context.Background()
	for i := 0; i < total; i++ {
		result, ok := r.Next(ctx)
		if !ok {
			t.Fatalf("Next returned false at iteration %d", i)
		}
		counts[result.Event.Priority]++
	}

	assertRatio(t, "high", counts["high"], total, 3.0/6.0, 0.05)
	assertRatio(t, "normal", counts["normal"], total, 2.0/6.0, 0.05)
	assertRatio(t, "low", counts["low"], total, 1.0/6.0, 0.05)
}

func assertRatio(t *testing.T, name string, got, total int, want, tol float64) {
	t.Helper()
	ratio := float64(got) / float64(total)
	if ratio < want-tol || ratio > want+tol {
		t.Errorf("%s ratio = %.3f, want %.3f ±%.3f", name, ratio, want, tol)
	}
}

func TestFallbackHighEmpty(t *testing.T) {
	r := priorityrouter.NewPriorityRouter(
		context.Background(),
		emptyChan(),
		filledChan([]stream.Result[stream.NotificationCreatedEvent]{makeResult("normal")}),
		emptyChan(),
		[3]int{3, 2, 1},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, ok := r.Next(ctx)
	if !ok {
		t.Fatal("expected a result, got false")
	}
	if result.Event.Priority != "normal" {
		t.Errorf("expected normal, got %s", result.Event.Priority)
	}
}

func TestFallbackAllEmpty(t *testing.T) {
	r := priorityrouter.NewPriorityRouter(
		context.Background(),
		emptyChan(), emptyChan(), emptyChan(),
		[3]int{3, 2, 1},
	)

	start := time.Now()
	_, ok := r.Next(context.Background())
	elapsed := time.Since(start)

	if ok {
		t.Fatal("expected false when all queues empty")
	}
	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("expected ~1s block, got %v", elapsed)
	}
}

func TestConcurrentSafe(t *testing.T) {
	const msgCount = 300
	msgs := make([]stream.Result[stream.NotificationCreatedEvent], msgCount)
	for i := range msgs {
		msgs[i] = makeResult("high")
	}

	r := priorityrouter.NewPriorityRouter(
		context.Background(),
		filledChan(msgs), emptyChan(), emptyChan(),
		[3]int{3, 2, 1},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := make(chan stream.Result[stream.NotificationCreatedEvent], msgCount)
	done := make(chan struct{})

	for range 10 {
		go func() {
			for {
				res, ok := r.Next(ctx)
				if !ok {
					done <- struct{}{}
					return
				}
				results <- res
			}
		}()
	}

	exited := 0
	collected := 0
	for exited < 10 {
		select {
		case <-results:
			collected++
		case <-done:
			exited++
		}
	}
	for {
		select {
		case <-results:
			collected++
		default:
			goto tally
		}
	}
tally:
	if collected != msgCount {
		t.Errorf("consumed %d messages, want %d", collected, msgCount)
	}
}

func TestContextCancel(t *testing.T) {
	r := priorityrouter.NewPriorityRouter(
		context.Background(),
		emptyChan(), emptyChan(), emptyChan(),
		[3]int{3, 2, 1},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, ok := r.Next(ctx)
	elapsed := time.Since(start)

	if ok {
		t.Fatal("expected false on cancelled context")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Next should return quickly on cancelled ctx, took %v", elapsed)
	}
}
