package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/barkin/insider-notification/processor/internal/worker"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
)

// --- fakes ---

type fakePublisher struct {
	mu   sync.Mutex
	sent []publishedMsg
}

type publishedMsg struct {
	topic   string
	payload any
}

func (f *fakePublisher) Publish(_ context.Context, topic string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, publishedMsg{topic, payload})
	return nil
}

func (f *fakePublisher) calls() []publishedMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]publishedMsg, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakePublisher) topicsPublished() []string {
	calls := f.calls()
	topics := make([]string, len(calls))
	for i, c := range calls {
		topics[i] = c.topic
	}
	return topics
}

type fakeCancellationStore struct{ cancelled bool }

func (f *fakeCancellationStore) IsCancelled(_ context.Context, _ string) (bool, error) {
	return f.cancelled, nil
}

type fakeLocker struct{ locked bool }

func (f *fakeLocker) TryLock(_ context.Context, _ string) (bool, error) { return f.locked, nil }
func (f *fakeLocker) Unlock(_ context.Context, _ string) error          { return nil }

// --- helpers ---

func newResult(evt stream.NotificationCreatedEvent) stream.Result[stream.NotificationCreatedEvent] {
	msg := message.NewMessage(watermill.NewUUID(), nil)
	return stream.Result[stream.NotificationCreatedEvent]{Event: evt, Msg: msg}
}

func baseEvent() stream.NotificationCreatedEvent {
	return stream.NotificationCreatedEvent{
		NotificationID: "notif-1",
		Priority:       model.PriorityHigh,
		Channel:        model.ChannelEmail,
		AttemptNumber:  1,
		MaxAttempts:    4,
	}
}

func newWorker(pub *fakePublisher, cancelled bool, lockGranted bool) *worker.Worker {
	return worker.NewWorker(
		pub,
		nil, // delivery client — not called in current stub
		nil, // rate limiter — skipped
		&fakeLocker{locked: lockGranted},
		&fakeCancellationStore{cancelled: cancelled},
	)
}

func isAcked(msg *message.Message) bool {
	select {
	case <-msg.Acked():
		return true
	default:
		return false
	}
}

func isNacked(msg *message.Message) bool {
	select {
	case <-msg.Nacked():
		return true
	default:
		return false
	}
}

// --- tests ---

// deliver_after in the future: re-enqueues to same topic and ACKs; no status published.
func TestProcessOne_DeliverAfterFuture(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, false, true)

	evt := baseEvent()
	evt.DeliverAfter = time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	result := newResult(evt)

	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	topics := pub.topicsPublished()
	if len(topics) != 1 || topics[0] != stream.TopicHigh {
		t.Errorf("expected re-enqueue to %s, got %v", stream.TopicHigh, topics)
	}
}

// deliver_after already passed: pipeline continues normally.
func TestProcessOne_DeliverAfterPast(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, false, true)

	evt := baseEvent()
	evt.DeliverAfter = time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	result := newResult(evt)

	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	topics := pub.topicsPublished()
	// processing + failed (stub delivery always fails terminally)
	if len(topics) != 2 {
		t.Errorf("expected 2 status publishes, got %d: %v", len(topics), topics)
	}
}

// Cancelled notification: ACKs immediately, no status published.
func TestProcessOne_Cancelled(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, true, true)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Lock miss: ACKs immediately, no status published.
func TestProcessOne_LockMiss(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, false, false)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Full pipeline with stub delivery: publishes processing then failed.
func TestProcessOne_TerminalFailure(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, false, true)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	topics := pub.topicsPublished()
	if len(topics) != 2 {
		t.Fatalf("expected 2 publishes, got %d: %v", len(topics), topics)
	}
	for _, topic := range topics {
		if topic != stream.TopicStatus {
			t.Errorf("expected all publishes to %s, got %s", stream.TopicStatus, topic)
		}
	}

	calls := pub.calls()
	first := calls[0].payload.(stream.NotificationDeliveryResultEvent)
	second := calls[1].payload.(stream.NotificationDeliveryResultEvent)

	if first.Status != model.StatusProcessing {
		t.Errorf("first publish: expected %s, got %s", model.StatusProcessing, first.Status)
	}
	if second.Status != model.StatusFailed {
		t.Errorf("second publish: expected %s, got %s", model.StatusFailed, second.Status)
	}
}

// singleSource feeds exactly one message then returns false forever.
type singleSource struct {
	result stream.Result[stream.NotificationCreatedEvent]
	done   bool
}

func (s *singleSource) Next(_ context.Context) (stream.Result[stream.NotificationCreatedEvent], bool) {
	if s.done {
		return stream.Result[stream.NotificationCreatedEvent]{}, false
	}
	s.done = true
	return s.result, true
}
