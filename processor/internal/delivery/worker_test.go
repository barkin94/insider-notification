package delivery_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/service"
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

type fakeLocker struct{ locked bool }

func (f *fakeLocker) TryLock(_ context.Context, _ string) (bool, error) { return f.locked, nil }
func (f *fakeLocker) Unlock(_ context.Context, _ string) error          { return nil }

type fakeDeliveryClient struct {
	result service.DeliveryResult
}

func (f *fakeDeliveryClient) Send(_ context.Context, _, _, _ string) service.DeliveryResult {
	return f.result
}

type fakeLimiter struct{ allowed bool }

func (f *fakeLimiter) IsAllowed(_ context.Context, _ string) (bool, time.Duration, error) {
	return f.allowed, 0, nil
}

type delayCall struct {
	notifID    string
	retryAfter time.Time
}

type mockAttemptWriter struct {
	mu           sync.Mutex
	calls        []*processordb.DeliveryAttempt
	delayCalls   []delayCall
	payloadCalls []*processordb.DeliveryAttempt
	err          error
	countResult  int
}

var _ processordb.DeliveryAttemptRepository = (*mockAttemptWriter)(nil)

func (m *mockAttemptWriter) SavePayload(_ context.Context, a *processordb.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payloadCalls = append(m.payloadCalls, a)
	return nil
}

func (m *mockAttemptWriter) Create(_ context.Context, a *processordb.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, a)
	return m.err
}

func (m *mockAttemptWriter) Delay(_ context.Context, notifID string, retryAfter time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delayCalls = append(m.delayCalls, delayCall{notifID, retryAfter})
	return m.err
}

func (m *mockAttemptWriter) GetAttemptNumber(_ context.Context, _ string) (int, error) {
	return m.countResult, nil
}

func (m *mockAttemptWriter) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockAttemptWriter) GetDue(_ context.Context, _ time.Time, _ int) ([]*processordb.DeliveryAttempt, error) {
	return nil, nil
}

func (m *mockAttemptWriter) RemoveDue(_ context.Context, _ string) error {
	return nil
}

func (m *mockAttemptWriter) recorded() []*processordb.DeliveryAttempt {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*processordb.DeliveryAttempt, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *mockAttemptWriter) delayed() []delayCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]delayCall, len(m.delayCalls))
	copy(out, m.delayCalls)
	return out
}

func (m *mockAttemptWriter) payloads() []*processordb.DeliveryAttempt {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*processordb.DeliveryAttempt, len(m.payloadCalls))
	copy(out, m.payloadCalls)
	return out
}

// --- helpers ---

// runSingle delivers one event through p and returns the watermill message for Ack/Nack inspection.
func runSingle(p *delivery.NotificationDeliveryPipelineWorker, evt stream.NotificationReadyEvent) *message.Message {
	ctx := context.Background()
	msg := message.NewMessage(watermill.NewUUID(), nil)
	result := stream.Result[stream.NotificationReadyEvent]{
		Ctx:   ctx,
		Event: evt,
		Msg:   msg,
	}
	p.Run(ctx, result)
	return msg
}

const baseNotifID = "00000000-0000-0000-0000-000000000001"

func baseEvent() stream.NotificationReadyEvent {
	return stream.NotificationReadyEvent{
		NotificationID: baseNotifID,
		Priority:       string(string(model.PriorityHigh)),
		Channel:        string(model.ChannelEmail),
		Recipient:      "+905551234567",
		Content:        "Your message",
		MaxAttempts:    4,
	}
}

func baseEventWithID() stream.NotificationReadyEvent {
	evt := baseEvent()
	evt.NotificationID = uuid.New().String()
	return evt
}

func newPipeline(pub *fakePublisher, dc service.NtfnDeliveryClient, lim service.Limiter, lockGranted bool, attempts processordb.DeliveryAttemptRepository) *delivery.NotificationDeliveryPipelineWorker {
	m, _ := service.NewMetrics(nil)
	return delivery.NewNotificationDeliveryPipelineWorker(
		pub,
		dc,
		lim,
		&fakeLocker{locked: lockGranted},
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

// Lock miss: ACKs immediately, no status published.
func TestProcessOne_LockMiss(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, false, &mockAttemptWriter{})

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
	c := newPipeline(pub, dc, lim, true, &mockAttemptWriter{})

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
	if evt.Status != string(model.StatusFailed) {
		t.Errorf("expected status %s, got %s", string(model.StatusFailed), evt.Status)
	}
}

// Success result: one status event with status=delivered, no DB write.
func TestDelivery_delivered(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: true, StatusCode: 202, LatencyMS: 50}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true, &mockAttemptWriter{})

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (delivered), got %d", len(calls))
	}
	evt := calls[0].payload.(stream.NotificationDeliveryResultEvent)
	if evt.Status != string(model.StatusDelivered) {
		t.Errorf("expected status %s, got %s", string(model.StatusDelivered), evt.Status)
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
	c := newPipeline(pub, dc, lim, true, aw)

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
	if attempts[0].RetryAfter == nil {
		t.Error("expected retry_after to be set")
	}
	payloads := aw.payloads()
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload write, got %d", len(payloads))
	}
	if payloads[0].Priority != string(model.PriorityHigh) {
		t.Errorf("priority = %q, want high", payloads[0].Priority)
	}
}

// Retryable failure at max attempts: status=failed published, attempt written without retry_after.
func TestDelivery_exhausted_failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	// 3 prior attempts → currentAttempt=4=MaxAttempts → terminal
	aw := &mockAttemptWriter{countResult: 3}
	c := newPipeline(pub, dc, lim, true, aw)

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
	if last.Status != string(model.StatusFailed) {
		t.Errorf("expected status %s, got %s", string(model.StatusFailed), last.Status)
	}
	if last.AttemptNumber != 4 {
		t.Errorf("expected attempt_number 4, got %d", last.AttemptNumber)
	}
	if len(aw.recorded()) != 0 {
		t.Errorf("expected no attempt writes on exhaustion (cleanup via Delete), got %d", len(aw.recorded()))
	}
}

// Non-retryable failure: status=failed, no re-enqueue.
func TestDelivery_nonRetryable_failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true, &mockAttemptWriter{})

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (failed), got %d", len(calls))
	}
	last := calls[0].payload.(stream.NotificationDeliveryResultEvent)
	if last.Status != string(model.StatusFailed) {
		t.Errorf("expected status %s, got %s", string(model.StatusFailed), last.Status)
	}
}

// Lock miss: ACKs, no publishes.
func TestDelivery_lockMiss(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, false, &mockAttemptWriter{})

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Rate limited: defers via Delay (not Create), no direct stream publish.
func TestDelivery_rateLimited_requeued(t *testing.T) {
	pub := &fakePublisher{}
	lim := &fakeLimiter{allowed: false}
	aw := &mockAttemptWriter{}
	c := newPipeline(pub, nil, lim, true, aw)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no direct publishes, got %v", pub.topicsPublished())
	}
	if len(aw.recorded()) != 0 {
		t.Errorf("expected no Create calls for rate-limited retry, got %d", len(aw.recorded()))
	}
	delayed := aw.delayed()
	if len(delayed) != 1 {
		t.Fatalf("expected 1 Delay call for rate-limited retry, got %d", len(delayed))
	}
	if delayed[0].retryAfter.IsZero() {
		t.Error("expected retry_after to be set for rate-limited retry")
	}
}

// --- delivery attempt tests ---

// Retryable failure path: attempt written with correct attempt number from count.
func TestDelivery_attempts_retryable(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	aw := &mockAttemptWriter{countResult: 0} // first attempt
	c := newPipeline(pub, dc, lim, true, aw)

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

// Terminal failure path: no attempt written via Create; state cleaned up via Delete, status published.
func TestDelivery_attempts_terminal(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	aw := &mockAttemptWriter{}
	c := newPipeline(pub, dc, lim, true, aw)

	msg := runSingle(c, baseEventWithID())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	if len(aw.recorded()) != 0 {
		t.Errorf("expected no attempt writes for terminal failure, got %d", len(aw.recorded()))
	}
	calls := pub.calls()
	if len(calls) != 1 || calls[0].topic != stream.TopicStatus {
		t.Errorf("expected status publish, got %v", pub.topicsPublished())
	}
}

// Write error on terminal failure: consumer still ACKs and publishes status.
func TestDelivery_attempts_writeError_doesNotAbort(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	aw := &mockAttemptWriter{err: errTest}
	c := newPipeline(pub, dc, lim, true, aw)

	msg := runSingle(c, baseEventWithID())

	if !isAcked(msg) {
		t.Error("expected ACK even when attempt write fails")
	}
	calls := pub.calls()
	if len(calls) != 1 || calls[0].topic != stream.TopicStatus {
		t.Errorf("expected status publish after write error, got %v", pub.topicsPublished())
	}
}

// Notification payload is persisted via SavePayload so the retry dispatcher can re-publish without a DB lookup.
func TestDelivery_attempts_payloadPersisted(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	aw := &mockAttemptWriter{countResult: 0}
	c := newPipeline(pub, dc, lim, true, aw)

	evt := baseEventWithID()
	evt.Channel = string(model.ChannelSMS)
	evt.Recipient = "+905550001"
	evt.Content = "hello"
	evt.MaxAttempts = 3
	runSingle(c, evt)

	payloads := aw.payloads()
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload write (SavePayload), got %d", len(payloads))
	}
	p := payloads[0]
	if p.Channel != string(model.ChannelSMS) {
		t.Errorf("channel = %q, want sms", p.Channel)
	}
	if p.Recipient != "+905550001" {
		t.Errorf("recipient = %q, want +905550001", p.Recipient)
	}
	if p.Content != "hello" {
		t.Errorf("content = %q, want hello", p.Content)
	}
	if p.MaxAttempts != 3 {
		t.Errorf("max_attempts = %d, want 3", p.MaxAttempts)
	}
}

var errTest = errors.New("simulated write error")
