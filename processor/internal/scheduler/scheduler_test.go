package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	processordb "github.com/barkin/insider-notification/processor/internal/db"
	"github.com/barkin/insider-notification/processor/internal/scheduler"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
)

// --- fakes ---

type fakePublisher struct {
	mu    sync.Mutex
	calls []publishedMsg
}

type publishedMsg struct {
	topic   string
	payload stream.NotificationCreatedEvent
}

func (f *fakePublisher) Publish(_ context.Context, topic string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, publishedMsg{topic, payload.(stream.NotificationCreatedEvent)})
	return nil
}

func (f *fakePublisher) published() []publishedMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]publishedMsg, len(f.calls))
	copy(out, f.calls)
	return out
}

type fakeRetryRepo struct {
	rows []*processordb.DeliveryAttempt
}

func (f *fakeRetryRepo) FindDueRetries(_ context.Context) ([]*processordb.DeliveryAttempt, error) {
	return f.rows, nil
}

type fakeNotifReader struct {
	rows   []scheduler.NotificationRow
	byID   map[string]scheduler.NotificationRow
}

func (f *fakeNotifReader) FindScheduledDue(_ context.Context) ([]scheduler.NotificationRow, error) {
	return f.rows, nil
}

func (f *fakeNotifReader) FindByIDs(_ context.Context, ids []uuid.UUID) ([]scheduler.NotificationRow, error) {
	var out []scheduler.NotificationRow
	for _, id := range ids {
		if row, ok := f.byID[id.String()]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

// --- tests ---

func TestTick_initialScheduled_enqueues(t *testing.T) {
	pub := &fakePublisher{}
	id := uuid.New()
	notifReader := &fakeNotifReader{rows: []scheduler.NotificationRow{
		{ID: id, Priority: model.PriorityHigh, Channel: model.ChannelSMS,
			Recipient: "+1", Content: "hello", MaxAttempts: 4},
	}}
	retryRepo := &fakeRetryRepo{}

	sched := scheduler.New(notifReader, retryRepo, pub)
	sched.Tick(context.Background())

	msgs := pub.published()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(msgs))
	}
	evt := msgs[0].payload
	if evt.NotificationID != id.String() {
		t.Errorf("notification_id = %q, want %q", evt.NotificationID, id.String())
	}
	if evt.AttemptNumber != 1 {
		t.Errorf("attempt_number = %d, want 1", evt.AttemptNumber)
	}
	if evt.DeliverAfter != "" {
		t.Errorf("deliver_after should be empty, got %q", evt.DeliverAfter)
	}
	if msgs[0].topic != stream.TopicHigh {
		t.Errorf("topic = %q, want %q", msgs[0].topic, stream.TopicHigh)
	}
}

func TestTick_initialScheduled_noRows(t *testing.T) {
	pub := &fakePublisher{}
	sched := scheduler.New(&fakeNotifReader{}, &fakeRetryRepo{}, pub)
	sched.Tick(context.Background())

	if len(pub.published()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.published()))
	}
}

func TestTick_retry_enqueuesWithNextAttempt(t *testing.T) {
	pub := &fakePublisher{}
	id := uuid.New()
	past := time.Now().Add(-time.Minute)
	retryRepo := &fakeRetryRepo{rows: []*processordb.DeliveryAttempt{
		{NotificationID: id, AttemptNumber: 2, Status: "failed",
			Priority: model.PriorityNormal, RetryAfter: &past},
	}}
	notifReader := &fakeNotifReader{byID: map[string]scheduler.NotificationRow{
		id.String(): {ID: id, Priority: model.PriorityNormal, Channel: model.ChannelEmail,
			Recipient: "+1", Content: "retry me", MaxAttempts: 4},
	}}

	sched := scheduler.New(notifReader, retryRepo, pub)
	sched.Tick(context.Background())

	msgs := pub.published()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(msgs))
	}
	evt := msgs[0].payload
	if evt.NotificationID != id.String() {
		t.Errorf("notification_id = %q, want %q", evt.NotificationID, id.String())
	}
	if evt.AttemptNumber != 3 {
		t.Errorf("attempt_number = %d, want 3", evt.AttemptNumber)
	}
	if evt.Channel != model.ChannelEmail {
		t.Errorf("channel = %q, want email", evt.Channel)
	}
	if evt.DeliverAfter != "" {
		t.Errorf("deliver_after should be empty, got %q", evt.DeliverAfter)
	}
	if msgs[0].topic != stream.TopicNormal {
		t.Errorf("topic = %q, want %q", msgs[0].topic, stream.TopicNormal)
	}
}

func TestTick_retry_noRows(t *testing.T) {
	pub := &fakePublisher{}
	sched := scheduler.New(&fakeNotifReader{}, &fakeRetryRepo{}, pub)
	sched.Tick(context.Background())

	if len(pub.published()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.published()))
	}
}
