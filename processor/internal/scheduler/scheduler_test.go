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
	payload stream.NotificationReadyEvent
}

func (f *fakePublisher) Publish(_ context.Context, topic string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, publishedMsg{topic, payload.(stream.NotificationReadyEvent)})
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

func (f *fakeRetryRepo) Create(_ context.Context, _ *processordb.DeliveryAttempt) error { return nil }

func (f *fakeRetryRepo) CountByNotificationID(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

// --- tests ---

func TestTick_retry_enqueuesWithNextAttempt(t *testing.T) {
	pub := &fakePublisher{}
	id := uuid.New()
	past := time.Now().Add(-time.Minute)
	retryRepo := &fakeRetryRepo{rows: []*processordb.DeliveryAttempt{
		{
			NotificationID: id,
			AttemptNumber:  2,
			Priority:       model.PriorityNormal,
			RetryAfter:     &past,
			Channel:        model.ChannelEmail,
			Recipient:      "+1",
			Content:        "retry me",
			MaxAttempts:    4,
			Metadata:       "{}",
		},
	}}

	sched := scheduler.New(retryRepo, pub, time.Second)
	sched.Tick(context.Background())

	msgs := pub.published()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(msgs))
	}
	evt := msgs[0].payload
	if evt.NotificationID != id.String() {
		t.Errorf("notification_id = %q, want %q", evt.NotificationID, id.String())
	}
	if evt.Channel != model.ChannelEmail {
		t.Errorf("channel = %q, want email", evt.Channel)
	}
	if msgs[0].topic != stream.TopicNormal {
		t.Errorf("topic = %q, want %q", msgs[0].topic, stream.TopicNormal)
	}
}

func TestTick_retry_noRows(t *testing.T) {
	pub := &fakePublisher{}
	sched := scheduler.New(&fakeRetryRepo{}, pub, time.Second)
	sched.Tick(context.Background())

	if len(pub.published()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.published()))
	}
}
