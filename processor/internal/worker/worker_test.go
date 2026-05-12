package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/barkin/insider-notification/processor/internal/worker"
	"github.com/barkin/insider-notification/processor/internal/worker/webhook"
	"github.com/barkin/insider-notification/processor/internal/worker/ratelimit"
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

type fakeDeliveryClient struct {
	result webhook.Result
	err    error
}

func (f *fakeDeliveryClient) Send(_ context.Context, _, _, _ string) (webhook.Result, error) {
	return f.result, f.err
}

type fakeLimiter struct{ allowed bool }

func (f *fakeLimiter) Allow(_ context.Context, _ string) (bool, error) {
	return f.allowed, nil
}

// --- helpers ---

func newResult(evt stream.NotificationCreatedEvent) stream.Result[stream.NotificationCreatedEvent] {
	msg := message.NewMessage(watermill.NewUUID(), nil)
	return stream.Result[stream.NotificationCreatedEvent]{
		Ctx:   context.Background(),
		Event: evt,
		Msg:   msg,
	}
}

func baseEvent() stream.NotificationCreatedEvent {
	return stream.NotificationCreatedEvent{
		NotificationID: "notif-1",
		Priority:       model.PriorityHigh,
		Channel:        model.ChannelEmail,
		Recipient:      "+905551234567",
		Content:        "Your message",
		AttemptNumber:  1,
		MaxAttempts:    4,
	}
}

func newWorker(pub *fakePublisher, dc webhook.Client, lim ratelimit.Limiter, cancelled bool, lockGranted bool) *worker.Worker {
	return worker.NewWorker(
		pub,
		dc,
		lim,
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

// --- tests ---

// deliver_after in the future: re-enqueues to same topic and ACKs; no status published.
func TestProcessOne_DeliverAfterFuture(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, nil, nil, false, true)

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
	dc := &fakeDeliveryClient{result: webhook.Result{Success: false, Retryable: false}}
	lim := &fakeLimiter{allowed: true}
	w := newWorker(pub, dc, lim, false, true)

	evt := baseEvent()
	evt.DeliverAfter = time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	result := newResult(evt)

	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	topics := pub.topicsPublished()
	// failed only
	if len(topics) != 1 {
		t.Errorf("expected 1 status publish, got %d: %v", len(topics), topics)
	}
}

// Cancelled notification: ACKs immediately, no status published.
func TestProcessOne_Cancelled(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, nil, nil, true, true)

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
	w := newWorker(pub, nil, nil, false, false)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Full pipeline with terminal non-retryable failure: publishes failed only.
func TestProcessOne_TerminalFailure(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: webhook.Result{Success: false, Retryable: false}}
	lim := &fakeLimiter{allowed: true}
	w := newWorker(pub, dc, lim, false, true)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish, got %d: %v", len(calls), calls)
	}
	if calls[0].topic != stream.TopicStatus {
		t.Errorf("expected publish to %s, got %s", stream.TopicStatus, calls[0].topic)
	}
	evt := calls[0].payload.(stream.NotificationDeliveryResultEvent)
	if evt.Status != model.StatusFailed {
		t.Errorf("expected status %s, got %s", model.StatusFailed, evt.Status)
	}
}

// Success result: one status event with status=delivered.
func TestWorker_delivered(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: webhook.Result{Success: true, StatusCode: 202, LatencyMS: 50}}
	lim := &fakeLimiter{allowed: true}
	w := newWorker(pub, dc, lim, false, true)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (delivered), got %d", len(calls))
	}
	evt := calls[0].payload.(stream.NotificationDeliveryResultEvent)
	if evt.Status != model.StatusDelivered {
		t.Errorf("expected status %s, got %s", model.StatusDelivered, evt.Status)
	}
	if evt.HTTPStatusCode != 202 {
		t.Errorf("expected status code 202, got %d", evt.HTTPStatusCode)
	}
}

// Retryable failure with attempts remaining: status event + re-enqueue with incremented AttemptNumber and DeliverAfter.
func TestWorker_retryable_requeued(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: webhook.Result{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	w := newWorker(pub, dc, lim, false, true)

	result := newResult(baseEvent()) // AttemptNumber=1, MaxAttempts=4
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	// re-enqueue to priority topic only; no status events on retryable failure
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (re-enqueue), got %d: %v", len(calls), calls)
	}
	if calls[0].topic != stream.TopicHigh {
		t.Errorf("publish should be re-enqueue to %s, got %s", stream.TopicHigh, calls[0].topic)
	}
	retryEvt := calls[0].payload.(stream.NotificationCreatedEvent)
	if retryEvt.AttemptNumber != 2 {
		t.Errorf("re-enqueued AttemptNumber: expected 2, got %d", retryEvt.AttemptNumber)
	}
	if retryEvt.DeliverAfter == "" {
		t.Error("re-enqueued event should have DeliverAfter set")
	}
}

// Retryable failure at max attempts: status=failed, no re-enqueue.
func TestWorker_exhausted_failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: webhook.Result{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	w := newWorker(pub, dc, lim, false, true)

	evt := baseEvent()
	evt.AttemptNumber = 4
	evt.MaxAttempts = 4
	result := newResult(evt)
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (failed), got %d", len(calls))
	}
	if calls[0].topic != stream.TopicStatus {
		t.Errorf("expected publish to %s, got %s", stream.TopicStatus, calls[0].topic)
	}
	last := calls[0].payload.(stream.NotificationDeliveryResultEvent)
	if last.Status != model.StatusFailed {
		t.Errorf("expected status %s, got %s", model.StatusFailed, last.Status)
	}
}

// Non-retryable failure: status=failed, no re-enqueue.
func TestWorker_nonRetryable_failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: webhook.Result{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	w := newWorker(pub, dc, lim, false, true)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (failed), got %d", len(calls))
	}
	last := calls[0].payload.(stream.NotificationDeliveryResultEvent)
	if last.Status != model.StatusFailed {
		t.Errorf("expected status %s, got %s", model.StatusFailed, last.Status)
	}
}

// Lock miss: ACKs, no publishes.
func TestWorker_lockMiss(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, nil, nil, false, false)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Future DeliverAfter: re-enqueue only, no status event.
func TestWorker_deliverAfter_requeued(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, nil, nil, false, true)

	evt := baseEvent()
	evt.DeliverAfter = time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	result := newResult(evt)
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (re-enqueue), got %d", len(calls))
	}
	if calls[0].topic != stream.TopicHigh {
		t.Errorf("expected re-enqueue to %s, got %s", stream.TopicHigh, calls[0].topic)
	}
	_, isStatus := calls[0].payload.(stream.NotificationDeliveryResultEvent)
	if isStatus {
		t.Error("expected re-enqueue payload, not status event")
	}
}

// Rate limited: re-enqueue with same AttemptNumber, no status event.
func TestWorker_rateLimited_requeued(t *testing.T) {
	pub := &fakePublisher{}
	lim := &fakeLimiter{allowed: false}
	w := newWorker(pub, nil, lim, false, true)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (re-enqueue), got %d", len(calls))
	}
	if calls[0].topic != stream.TopicHigh {
		t.Errorf("expected re-enqueue to %s, got %s", stream.TopicHigh, calls[0].topic)
	}
	retryEvt, ok := calls[0].payload.(stream.NotificationCreatedEvent)
	if !ok {
		t.Fatal("expected re-enqueue payload to be NotificationCreatedEvent")
	}
	if retryEvt.AttemptNumber != 1 {
		t.Errorf("rate-limited re-enqueue should keep same AttemptNumber=1, got %d", retryEvt.AttemptNumber)
	}
}

// Cancelled notification: ACKs immediately, no publishes.
func TestWorker_cancelled(t *testing.T) {
	pub := &fakePublisher{}
	w := newWorker(pub, nil, nil, true, true)

	result := newResult(baseEvent())
	w.Run(context.Background(), &singleSource{result: result})

	if !isAcked(result.Msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
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
