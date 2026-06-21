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

	"github.com/barkin/insider-notification/processor/internal/delivery"
	"github.com/barkin/insider-notification/processor/internal/service"
	apipub "github.com/barkin/insider-notification/api/public"
	processorpub "github.com/barkin/insider-notification/processor/public"
	stream "github.com/barkin/insider-notification/shared/messaging"
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

type fakeLimiter struct{ allowed bool }

func (f *fakeLimiter) IsAllowed(_ context.Context, _ string) (bool, time.Duration, error) {
	return f.allowed, 0, nil
}

// --- helpers ---

func runSingle(p *delivery.NotificationDeliveryPipeline, evt apipub.NotificationReadyEvent) *message.Message {
	ctx := context.Background()
	msg := message.NewMessage(watermill.NewUUID(), nil)
	result := stream.Result[apipub.NotificationReadyEvent]{
		Ctx:   ctx,
		Event: evt,
		Msg:   msg,
	}
	_ = p.Run(ctx, result)
	return msg
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
func TestPipeline_LockMiss(t *testing.T) {
	pub := &fakePublisher{}
	c := newPipeline(pub, nil, nil, false)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
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

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
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

// Success result: one status event with status=delivered, no retry publish.
func TestPipeline_Delivered(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: true, StatusCode: 202, LatencyMS: 50}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
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

// Retryable failure with attempts remaining: publishes NotificationRetryScheduleEvent to TopicRetry.
func TestPipeline_Retryable_PublishesRetry(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	before := time.Now()
	msg := runSingle(c, baseEventWithID())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 publish (retry), got %d: %v", len(calls), pub.topicsPublished())
	}
	if calls[0].topic != processorpub.TopicRetry {
		t.Errorf("expected publish to %s, got %s", processorpub.TopicRetry, calls[0].topic)
	}
	retryEvt := calls[0].payload.(processorpub.NotificationRetryScheduleEvent)
	if retryEvt.ScheduledAt.Before(before) {
		t.Error("expected ScheduledAt to be in the future")
	}
	if retryEvt.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", retryEvt.AttemptNumber)
	}
	if retryEvt.Priority != string(apipub.PriorityHigh) {
		t.Errorf("Priority = %q, want high", retryEvt.Priority)
	}
}

// Retryable failure at max attempts: status=failed published, no retry publish.
func TestPipeline_Exhausted_Failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	// 3 prior attempts → currentAttempt=4=MaxAttempts → terminal
	evt := baseEventWithID()
	evt.MaxAttempts = 4
	evt.AttemptNumber = 3
	msg := runSingle(c, evt)

	if !isAcked(msg) {
		t.Error("expected ACK")
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

// Non-retryable failure: status=failed, no retry publish.
func TestPipeline_NonRetryable_Failed(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
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

// Rate limited: publishes NotificationRetryScheduleEvent to TopicRetry, no delivery attempted.
func TestPipeline_RateLimited_PublishesRetry(t *testing.T) {
	pub := &fakePublisher{}
	lim := &fakeLimiter{allowed: false}
	c := newPipeline(pub, nil, lim, true)

	before := time.Now()
	msg := runSingle(c, baseEvent())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 retry publish for rate-limited event, got %d", len(calls))
	}
	if calls[0].topic != processorpub.TopicRetry {
		t.Errorf("expected publish to %s, got %s", processorpub.TopicRetry, calls[0].topic)
	}
	retryEvt := calls[0].payload.(processorpub.NotificationRetryScheduleEvent)
	if retryEvt.ScheduledAt.Before(before) {
		t.Error("expected ScheduledAt to be in the future")
	}
	// Rate limit does not increment AttemptNumber — the attempt hasn't happened yet.
	if retryEvt.AttemptNumber != 0 {
		t.Errorf("AttemptNumber = %d, want 0 (no delivery was attempted)", retryEvt.AttemptNumber)
	}
}

// --- delivery attempt tests ---

// Retryable failure path: retry event carries correct AttemptNumber.
func TestPipeline_Attempts_Retryable(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	evt := baseEventWithID()
	evt.MaxAttempts = 4
	runSingle(c, evt)

	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 retry publish, got %d", len(calls))
	}
	retryEvt := calls[0].payload.(processorpub.NotificationRetryScheduleEvent)
	if retryEvt.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", retryEvt.AttemptNumber)
	}
}

// Terminal failure path: no retry publish, status published.
func TestPipeline_Attempts_Terminal(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: false, StatusCode: 400}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	msg := runSingle(c, baseEventWithID())

	if !isAcked(msg) {
		t.Error("expected ACK")
	}
	calls := pub.calls()
	if len(calls) != 1 || calls[0].topic != processorpub.TopicStatus {
		t.Errorf("expected status publish only, got %v", pub.topicsPublished())
	}
}

// Retry publish error causes Nack so the message is redelivered.
func TestPipeline_RetryPublishError_Nacks(t *testing.T) {
	pub := &fakePublisher{errOn: processorpub.TopicRetry}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	msg := runSingle(c, baseEventWithID())

	if isAcked(msg) {
		t.Error("expected Nack when retry publish fails")
	}
}

// Retry event carries the full notification payload so retryscheduler can republish without a DB lookup.
func TestPipeline_Attempts_PayloadCarried(t *testing.T) {
	pub := &fakePublisher{}
	dc := &fakeDeliveryClient{result: service.DeliveryResult{Success: false, Retryable: true, StatusCode: 503}}
	lim := &fakeLimiter{allowed: true}
	c := newPipeline(pub, dc, lim, true)

	evt := baseEventWithID()
	evt.Channel = string(apipub.ChannelSMS)
	evt.Recipient = "+905550001"
	evt.Content = "hello"
	evt.MaxAttempts = 3
	runSingle(c, evt)

	calls := pub.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 retry publish, got %d", len(calls))
	}
	retryEvt := calls[0].payload.(processorpub.NotificationRetryScheduleEvent)
	if retryEvt.Channel != string(apipub.ChannelSMS) {
		t.Errorf("Channel = %q, want sms", retryEvt.Channel)
	}
	if retryEvt.Recipient != "+905550001" {
		t.Errorf("Recipient = %q, want +905550001", retryEvt.Recipient)
	}
	if retryEvt.Content != "hello" {
		t.Errorf("Content = %q, want hello", retryEvt.Content)
	}
	if retryEvt.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", retryEvt.MaxAttempts)
	}
}

var errTest = errors.New("simulated error")
