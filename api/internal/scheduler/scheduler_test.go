package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/barkin/insider-notification/api/internal/db/entities"
	apischeduler "github.com/barkin/insider-notification/api/internal/scheduler"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
)

// --- fakes ---

type fakeRepo struct {
	rows []*entities.Notification
}

func (f *fakeRepo) FindScheduledDue(_ context.Context) ([]*entities.Notification, error) {
	return f.rows, nil
}

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

func makeNotif(priority, channel string) *entities.Notification {
	n := &entities.Notification{
		Recipient:   "+15551234567",
		Channel:     channel,
		Content:     "hello",
		Priority:    priority,
		MaxAttempts: 4,
	}
	n.ID = uuid.New()
	n.CreatedAt = time.Now()
	n.UpdatedAt = time.Now()
	return n
}

// --- tests ---

func TestTick_scheduledDue_published(t *testing.T) {
	pub := &fakePublisher{}
	n := makeNotif(string(model.PriorityHigh), string(model.ChannelSMS))
	repo := &fakeRepo{rows: []*entities.Notification{n}}

	sched := apischeduler.New(repo, pub, time.Second)
	sched.Tick(context.Background())

	msgs := pub.published()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(msgs))
	}
	evt := msgs[0].payload
	if evt.NotificationID != n.ID.String() {
		t.Errorf("notification_id = %q, want %q", evt.NotificationID, n.ID.String())
	}
	if evt.Channel != string(model.ChannelSMS) {
		t.Errorf("channel = %q, want sms", evt.Channel)
	}
	if msgs[0].topic != stream.TopicHigh {
		t.Errorf("topic = %q, want %q", msgs[0].topic, stream.TopicHigh)
	}
}

func TestTick_noRows_noPublish(t *testing.T) {
	pub := &fakePublisher{}
	sched := apischeduler.New(&fakeRepo{}, pub, time.Second)
	sched.Tick(context.Background())

	if len(pub.published()) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.published()))
	}
}

func TestTick_multipleNotifications_allPublished(t *testing.T) {
	pub := &fakePublisher{}
	notifications := []*entities.Notification{
		makeNotif(string(model.PriorityHigh), string(model.ChannelSMS)),
		makeNotif(string(model.PriorityNormal), string(model.ChannelEmail)),
		makeNotif(string(model.PriorityLow), string(model.ChannelPush)),
	}
	repo := &fakeRepo{rows: notifications}

	sched := apischeduler.New(repo, pub, time.Second)
	sched.Tick(context.Background())

	msgs := pub.published()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 publishes, got %d", len(msgs))
	}
	topics := map[string]bool{}
	for _, m := range msgs {
		topics[m.topic] = true
	}
	if !topics[stream.TopicHigh] || !topics[stream.TopicNormal] || !topics[stream.TopicLow] {
		t.Errorf("missing expected topics, got %v", topics)
	}
}
