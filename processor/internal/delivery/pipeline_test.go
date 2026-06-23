package delivery_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	apipub "github.com/barkin94/insider-notification/api/public"
	"github.com/barkin94/insider-notification/processor/internal/delivery"
	"github.com/barkin94/insider-notification/processor/internal/service"
	processorpub "github.com/barkin94/insider-notification/processor/public"
)

// --- fakes ---

type fakePublisher struct {
	mu    sync.Mutex
	sent  []publishedMsg
	errOn string // return errTest when publishing to this topic
}

type publishedMsg struct {
	topic   string
	payload any
}

func (f *fakePublisher) Publish(_ context.Context, topic string, payload any) error {
	if f.errOn != "" && topic == f.errOn {
		return errTest
	}
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

type fakeLimiter struct {
	allowed    bool
	retryAfter time.Duration
}

func (f *fakeLimiter) IsAllowed(_ context.Context, _ string) (bool, time.Duration, error) {
	return f.allowed, f.retryAfter, nil
}

// --- helpers ---

// runSingle runs the pipeline with attemptNumber=1 (first delivery).
func runSingle(p *delivery.NotificationDeliveryPipeline, evt apipub.NotificationReadyEvent) error {
	return p.Run(context.Background(), evt, 1)
}

func runWithCount(p *delivery.NotificationDeliveryPipeline, evt apipub.NotificationReadyEvent, attemptNumber int) error {
	return p.Run(context.Background(), evt, attemptNumber)
}

const baseNotifID = "00000000-0000-0000-0000-000000000001"

func baseEvent() apipub.NotificationReadyEvent {
	return apipub.NotificationReadyEvent{
		NotificationID: baseNotifID,
		Priority:       string(apipub.PriorityHigh),
		Channel:        string(apipub.ChannelEmail),
		Recipient:      "+905551234567",
		Content:        "Your message",
		MaxAttempts:    4,
	}
}

func baseEventWithID() apipub.NotificationReadyEvent {
	evt := baseEvent()
	evt.NotificationID = uuid.New().String()
	return evt
}

func newPipeline(pub *fakePublisher, dc service.NtfnDeliveryClient, lim service.Limiter, lockGranted bool) *delivery.NotificationDeliveryPipeline {
	m, _ := service.NewMetrics(nil)
	return delivery.NewNotificationDeliveryPipeline(
		pub,
		dc,
		lim,
		&fakeLocker{locked: lockGranted},
		m,
	)
}

func isErrRetryAfter(err error) (delivery.ErrRetryAfter, bool) {
	var e delivery.ErrRetryAfter
	return e, errors.As(err, &e)
}

// --- tests ---

// Lock miss: returns nil (Ack), no status published.
func TestPipeline_LockMiss(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, false)

	if err := runSingle(c, baseEvent()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.calls()))
	}
}

// Full pipeline with terminal non-retryable failure: publishes failed status only.
func TestPipeline_TerminalFailure(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	err := runSingle(c, baseEvent())
	if err != nil {
		t.Errorf("expected nil (Ack path), got %v", err)
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish, got %d: %v", len(calls), calls)
	}
	if calls[0].topic != processorpub.TopicStatus {
		t.Errorf("expected publish to %s, got %s", processorpub.TopicStatus, calls[0].topic)
	}
	evt := calls[0].payload.(processorpub.NotificationDeliveryResultEvent)
	if evt.Status != string(apipub.StatusFailed) {
		t.Errorf("expected status %s, got %s", string(apipub.StatusFailed), evt.Status)
	}
}

// Success result: one status event with status=delivered, no retry.
func TestPipeline_Delivered(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: true, StatusCode: 202, LatencyMS: 50}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	if err := runSingle(c, baseEvent()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (delivered), got %d", len(calls))
	}
	evt := calls[0].payload.(processorpub.NotificationDeliveryResultEvent)
	if evt.Status != string(apipub.StatusDelivered) {
		t.Errorf("expected status %s, got %s", string(apipub.StatusDelivered), evt.Status)
	}
	if evt.HTTPStatusCode != 202 {
		t.Errorf("expected status code 202, got %d", evt.HTTPStatusCode)
	}
}

// Retryable failure with attempts remaining: returns ErrRetryAfter (NakWithDelay path).
func TestPipeline_Retryable_ReturnsErrRetryAfter(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	err := runSingle(c, baseEventWithID())

	ra, ok := isErrRetryAfter(err)
	if !ok {
		t.Fatalf("expected ErrRetryAfter, got %v", err)
	}
	if ra.Delay <= 0 {
		t.Errorf("expected positive delay, got %s", ra.Delay)
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes on retry path, got %v", pub.topicsPublished())
	}
}

// Retryable failure at max attempts: returns nil (Ack), publishes status=failed.
func TestPipeline_Exhausted_Failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	// attemptNumber=4 = MaxAttempts → terminal
	evt := baseEventWithID()
	evt.MaxAttempts = 4
	err := runWithCount(c, evt, 4)

	if err != nil {
		t.Errorf("expected nil (Ack path), got %v", err)
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (failed), got %d", len(calls))
	}
	if calls[0].topic != processorpub.TopicStatus {
		t.Errorf("expected publish to %s, got %s", processorpub.TopicStatus, calls[0].topic)
	}
	last := calls[0].payload.(processorpub.NotificationDeliveryResultEvent)
	if last.Status != string(apipub.StatusFailed) {
		t.Errorf("expected status %s, got %s", string(apipub.StatusFailed), last.Status)
	}
	if last.AttemptNumber != 4 {
		t.Errorf("expected AttemptNumber 4, got %d", last.AttemptNumber)
	}
}

// Non-retryable failure: returns nil (Ack), publishes status=failed.
func TestPipeline_NonRetryable_Failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	if err := runSingle(c, baseEvent()); err != nil {
		t.Errorf("expected nil (Ack path), got %v", err)
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (failed), got %d", len(calls))
	}
	last := calls[0].payload.(processorpub.NotificationDeliveryResultEvent)
	if last.Status != string(apipub.StatusFailed) {
		t.Errorf("expected status %s, got %s", string(apipub.StatusFailed), last.Status)
	}
}

// Rate limited: returns ErrRetryAfter, no delivery attempted, no status published.
func TestPipeline_RateLimited_ReturnsErrRetryAfter(t *testing.T) {
	pub := &fakePublisher{}
	lim := &fakeLimiter{allowed: false, retryAfter: 2 * time.Second}
	c := newPipeline(pub, nil, lim, true)

	err := runSingle(c, baseEvent())

	ra, ok := isErrRetryAfter(err)
	if !ok {
		t.Fatalf("expected ErrRetryAfter, got %v", err)
	}
	if ra.Delay != 2*time.Second {
		t.Errorf("expected delay 2s, got %s", ra.Delay)
	}
	if len(pub.calls()) != 0 {
		t.Errorf("expected no publishes on rate-limit path, got %v", pub.topicsPublished())
	}
}

// Rate limit with zero retryAfter falls back to 1s minimum.
func TestPipeline_RateLimited_ZeroRetryAfter_DefaultsToOneSecond(t *testing.T) {
	pub := &fakePublisher{}
	lim := &fakeLimiter{allowed: false, retryAfter: 0}
	c := newPipeline(pub, nil, lim, true)

	err := runSingle(c, baseEvent())

	ra, ok := isErrRetryAfter(err)
	if !ok {
		t.Fatalf("expected ErrRetryAfter, got %v", err)
	}
	if ra.Delay != time.Second {
		t.Errorf("expected 1s fallback delay, got %s", ra.Delay)
	}
}

// --- delivery attempt number tests ---

// AttemptNumber in status event equals attemptNumber.
func TestPipeline_AttemptNumber_FromNATSMetadata(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: true}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	err := runWithCount(c, baseEventWithID(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(calls))
	}
	evt := calls[0].payload.(processorpub.NotificationDeliveryResultEvent)
	if evt.AttemptNumber != 3 {
		t.Errorf("AttemptNumber = %d, want 3", evt.AttemptNumber)
	}
}

// Retryable failure: ErrRetryAfter delay is positive (backoff formula applied).
func TestPipeline_Retryable_DelayIsPositive(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	err := runWithCount(c, baseEventWithID(), 2)
	ra, ok := isErrRetryAfter(err)
	if !ok {
		t.Fatalf("expected ErrRetryAfter, got %v", err)
	}
	if ra.Delay <= 0 {
		t.Errorf("expected positive delay, got %s", ra.Delay)
	}
}

// Status publish error causes a non-nil return so caller Nacks.
func TestPipeline_StatusPublishError_ReturnsError(t *testing.T) {
	pub := &fakePublisher{errOn: processorpub.TopicStatus}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: true}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	err := runSingle(c, baseEventWithID())
	if err == nil {
		t.Error("expected error when status publish fails")
	}
	if _, ok := isErrRetryAfter(err); ok {
		t.Error("expected plain error, not ErrRetryAfter")
	}
}

var errTest = errors.New("simulated error")
