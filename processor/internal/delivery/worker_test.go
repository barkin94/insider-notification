package delivery_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/service"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
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
	result service.DeliveryResult
	err    error
}

func (f *fakeDeliveryClient) Send(_ context.Context, _, _, _ string) (service.DeliveryResult, error) {
	return f.result, f.err
}

type fakeLimiter struct{ allowed bool }

func (f *fakeLimiter) Allow(_ context.Context, _ string) (bool, error) {
	return f.allowed, nil
}

type mockAttemptWriter struct {
	mu          sync.Mutex
	calls       []*processordb.DeliveryAttempt
	err         error
	countResult int
}

var _ processordb.DeliveryAttemptRepository = (*mockAttemptWriter)(nil)

func (m *mockAttemptWriter) Create(_ context.Context, a *processordb.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, a)
	return m.err
}

func (m *mockAttemptWriter) CountByNotificationID(_ context.Context, _ uuid.UUID) (int, error) {
	return m.countResult, nil
}

// FindDueRetries implements [db.DeliveryAttemptRepository].
func (m *mockAttemptWriter) FindDueRetries(ctx context.Context) ([]*processordb.DeliveryAttempt, error) {
	return nil, nil
	//panic("unimplemented")
}

func (m *mockAttemptWriter) recorded() []*processordb.DeliveryAttempt {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*processordb.DeliveryAttempt, len(m.calls))
	copy(out, m.calls)
	return out
}

// --- helpers ---

// runSingle delivers one event through p and returns the watermill message for Ack/Nack inspection.
func runSingle(p *delivery.NotificationDeliveryPipeline, evt stream.NotificationCreatedEvent) *message.Message {
	ctx := context.Background()
	msg := message.NewMessage(watermill.NewUUID(), nil)
	result := stream.Result[stream.NotificationCreatedEvent]{
		Ctx:   ctx,
		Event: evt,
		Msg:   msg,
	}
	p.Run(ctx, result)
	return msg
}

const baseNotifID = "00000000-0000-0000-0000-000000000001"

func baseEvent() stream.NotificationCreatedEvent {
	return stream.NotificationCreatedEvent{
		NotificationID: baseNotifID,
		Priority:       model.PriorityHigh,
		Channel:        model.ChannelEmail,
		Recipient:      "+905551234567",
		Content:        "Your message",
		MaxAttempts:    4,
	}
}

func baseEventWithID() stream.NotificationCreatedEvent {
	evt := baseEvent()
	evt.NotificationID = uuid.New().String()
	return evt
}

func newPipeline(pub *fakePublisher, dc service.NtfnDeliveryClient, lim service.Limiter, cancelled bool, lockGranted bool, attempts processordb.DeliveryAttemptRepository) *delivery.NotificationDeliveryPipeline {
	m, _ := service.NewMetrics(nil)
	return delivery.NewNotificationDeliveryPipeline(
		pub,
		dc,
		lim,
		&fakeLocker{locked: lockGranted},
		&fakeCancellationStore{cancelled: cancelled},
		attempts,
		m,
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

// deliver_after in the future: ACKs and drops; scheduler handles it.
func TestProcessOne_DeliverAfterFuture(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, false, true, nil)

	evt := baseEvent()
	evt.DeliverAfter = time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)

	msg := runSingle(c, evt)

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %v", pub.topicsPublished())
	}
}

// deliver_after already passed: pipeline continues normally.
func TestProcessOne_DeliverAfterPast(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, false, true, nil)

	evt := baseEvent()
	evt.DeliverAfter = time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)

	msg := runSingle(c, evt)

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	topics := pub.topicsPublished()
	if len(topics) != 1 {
		t.Errorf("expected 1 status publish, got %d: %v", len(topics), topics)
	}
}

// Cancelled notification: ACKs immediately, no status published.
func TestProcessOne_Cancelled(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, true, true, nil)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Lock miss: ACKs immediately, no status published.
func TestProcessOne_LockMiss(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, false, false, nil)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Full pipeline with terminal non-retryable failure: publishes failed only.
func TestProcessOne_TerminalFailure(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, false, true, nil)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
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

// Success result: one status event with status=delivered, no DB write.
func TestDelivery_delivered(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: true, StatusCode: 202, LatencyMS: 50}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, false, true, nil)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
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

// Retryable failure with attempts remaining: attempt written to DB with retry_after set; no stream publish.
func TestDelivery_retryable_writesRetryAfter(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	aw := &mockAttemptWriter{countResult: 0} // first attempt
	c := newPipeline(pub, dc, lim, false, true, aw)

	msg := runSingle(c, baseEventWithID())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no stream publishes, got %v", pub.topicsPublished())
	}
	attempts := aw.recorded()
	if len(attempts) != 1 {
		t.Fatalf("expected 1 attempt write, got %d", len(attempts))
	}
	a := attempts[0]
	if a.RetryAfter == nil {
		t.Error("expected retry_after to be set")
	}
	if a.Priority != model.PriorityHigh {
		t.Errorf("priority = %q, want high", a.Priority)
	}
}

// Retryable failure at max attempts: status=failed published, attempt written without retry_after.
func TestDelivery_exhausted_failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	// 3 prior attempts → currentAttempt=4=MaxAttempts → terminal
	aw := &mockAttemptWriter{countResult: 3}
	c := newPipeline(pub, dc, lim, false, true, aw)

	evt := baseEventWithID()
	evt.MaxAttempts = 4
	msg := runSingle(c, evt)

	if !isAcked(msg) {
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
	if last.AttemptNumber != 4 {
		t.Errorf("expected attempt_number 4, got %d", last.AttemptNumber)
	}
	written := aw.recorded()
	if len(written) != 1 {
		t.Fatalf("expected 1 attempt write (terminal tombstone), got %d", len(written))
	}
	if written[0].RetryAfter != nil {
		t.Error("expected retry_after to be nil for terminal attempt")
	}
}

// Non-retryable failure: status=failed, no re-enqueue.
func TestDelivery_nonRetryable_failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, false, true, nil)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
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
func TestDelivery_lockMiss(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, false, false, nil)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Future DeliverAfter: ACK and drop; scheduler handles it.
func TestDelivery_deliverAfter_dropped(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, false, true, nil)

	evt := baseEvent()
	evt.DeliverAfter = time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)

	msg := runSingle(c, evt)

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %v", pub.topicsPublished())
	}
}

// Rate limited: re-enqueues the event unchanged, no status event.
func TestDelivery_rateLimited_requeued(t *testing.T) {
	pub := &fakePublisher{}
	lim := &fakeLimiter{allowed: false}
	c := newPipeline(pub, nil, lim, false, true, nil)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (re-enqueue), got %d", len(calls))
	}
	if calls[0].topic != stream.TopicHigh {
		t.Errorf("expected re-enqueue to %s, got %s", stream.TopicHigh, calls[0].topic)
	}
	if _, ok := calls[0].payload.(stream.NotificationCreatedEvent); !ok {
		t.Fatal("expected re-enqueue payload to be NotificationCreatedEvent")
	}
}

// Cancelled notification: ACKs immediately, no publishes.
func TestDelivery_cancelled(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, true, true, nil)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// --- delivery attempt tests ---

// Retryable failure path: attempt written with status=failed and correct attempt number from count.
func TestDelivery_attempts_retryable(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	aw := &mockAttemptWriter{countResult: 0} // first attempt
	c := newPipeline(pub, dc, lim, false, true, aw)

	evt := baseEventWithID()
	evt.MaxAttempts = 4
	runSingle(c, evt)

	attempts := aw.recorded()
	if len(attempts) != 1 {
		t.Fatalf("expected 1 attempt write, got %d", len(attempts))
	}
	if attempts[0].AttemptNumber != 1 {
		t.Errorf("attempt number = %d, want 1", attempts[0].AttemptNumber)
	}
}

// Terminal failure path: attempt written with status=failed, retry_after nil.
func TestDelivery_attempts_terminal(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	aw := &mockAttemptWriter{}
	c := newPipeline(pub, dc, lim, false, true, aw)

	runSingle(c, baseEventWithID())

	attempts := aw.recorded()
	if len(attempts) != 1 {
		t.Fatalf("expected 1 attempt write, got %d", len(attempts))
	}
	if attempts[0].RetryAfter != nil {
		t.Error("expected retry_after to be nil for terminal attempt")
	}
}

// Write error on terminal failure: consumer still ACKs and publishes status.
func TestDelivery_attempts_writeError_doesNotAbort(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	aw := &mockAttemptWriter{err: errTest}
	c := newPipeline(pub, dc, lim, false, true, aw)

	msg := runSingle(c, baseEventWithID())

	if !isAcked(msg) {
		t.Error("expected ACK even when attempt write fails")
	}
	calls := pub.calls()
	if len(calls) != 1 || calls[0].topic != stream.TopicStatus {
		t.Errorf("expected status publish after write error, got %v", pub.topicsPublished())
	}
}

var errTest = errors.New("simulated write error")
