package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/api/internal/service"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
)

// --- mock repo ---

type mockNotifRepo struct {
	createFn       func(ctx context.Context, n *model.Notification) error
	getByIDFn      func(ctx context.Context, id uuid.UUID) (*model.Notification, error)
	listFn         func(ctx context.Context, f db.ListFilter) ([]*model.Notification, int, error)
	transitionFn   func(ctx context.Context, id uuid.UUID, from, to string) (*model.Notification, error)
	incrFn         func(ctx context.Context, id uuid.UUID) error
	updateStatusFn func(ctx context.Context, id uuid.UUID, status string) error
}

func (m *mockNotifRepo) Create(ctx context.Context, n *model.Notification) error {
	return m.createFn(ctx, n)
}
func (m *mockNotifRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockNotifRepo) List(ctx context.Context, f db.ListFilter) ([]*model.Notification, int, error) {
	return m.listFn(ctx, f)
}
func (m *mockNotifRepo) Transition(ctx context.Context, id uuid.UUID, from, to string) (*model.Notification, error) {
	return m.transitionFn(ctx, id, from, to)
}
func (m *mockNotifRepo) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	return m.incrFn(ctx, id)
}
func (m *mockNotifRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status)
	}
	return nil
}

// --- mock delivery attempt repo ---

type mockAttemptRepo struct{}

func (m *mockAttemptRepo) Create(ctx context.Context, a *model.DeliveryAttempt) error { return nil }
func (m *mockAttemptRepo) ListByNotificationID(ctx context.Context, id uuid.UUID) ([]*model.DeliveryAttempt, error) {
	return nil, nil
}

// --- mock publisher ---

type mockPublisher struct {
	publishFn func(ctx context.Context, topic string, payload any) error
}

func (m *mockPublisher) Publish(ctx context.Context, topic string, payload any) error {
	return m.publishFn(ctx, topic, payload)
}

// --- helpers ---

func okRepo() *mockNotifRepo {
	return &mockNotifRepo{
		createFn: func(_ context.Context, _ *model.Notification) error { return nil },
		transitionFn: func(_ context.Context, id uuid.UUID, _, to string) (*model.Notification, error) {
			return &model.Notification{ID: id, Status: to}, nil
		},
	}
}

func okPublisher(wantTopic *string) *mockPublisher {
	return &mockPublisher{
		publishFn: func(_ context.Context, topic string, _ any) error {
			if wantTopic != nil {
				*wantTopic = topic
			}
			return nil
		},
	}
}

func newSvc(repo db.NotificationRepository, pub service.StreamPublisher) service.NotificationService {
	return service.NewNotificationService(repo, &mockAttemptRepo{}, pub)
}

// --- tests ---

func TestCreate_success(t *testing.T) {
	var gotTopic string
	svc := newSvc(okRepo(), okPublisher(&gotTopic))

	n, err := svc.Create(context.Background(), service.CreateRequest{
		Recipient: "+905551234567",
		Channel:   model.ChannelSMS,
		Content:   "hello",
		Priority:  model.PriorityHigh,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Status != model.StatusPending {
		t.Errorf("status = %q, want pending", n.Status)
	}
	if gotTopic != stream.TopicHigh {
		t.Errorf("published to %q, want %q", gotTopic, stream.TopicHigh)
	}
}

func TestCreate_invalidChannel(t *testing.T) {
	svc := newSvc(okRepo(), okPublisher(nil))
	_, err := svc.Create(context.Background(), service.CreateRequest{
		Recipient: "+1",
		Channel:   "fax",
		Content:   "hi",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCreate_contentTooLong(t *testing.T) {
	svc := newSvc(okRepo(), okPublisher(nil))
	long := make([]byte, 1601)
	for i := range long {
		long[i] = 'x'
	}
	_, err := svc.Create(context.Background(), service.CreateRequest{
		Recipient: "+1",
		Channel:   model.ChannelSMS,
		Content:   string(long),
	})
	if err == nil {
		t.Fatal("expected content-too-long error")
	}
}

func TestCreate_publishFailure(t *testing.T) {
	pub := &mockPublisher{publishFn: func(_ context.Context, _ string, _ any) error {
		return errors.New("redis down")
	}}
	svc := newSvc(okRepo(), pub)
	_, err := svc.Create(context.Background(), service.CreateRequest{
		Recipient: "+1",
		Channel:   model.ChannelSMS,
		Content:   "hi",
	})
	if err == nil {
		t.Fatal("expected error from publisher")
	}
}

func TestCancel_success(t *testing.T) {
	var gotTopic string
	svc := newSvc(okRepo(), okPublisher(&gotTopic))
	n, err := svc.Cancel(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Status != model.StatusCancelled {
		t.Errorf("status = %q, want cancelled", n.Status)
	}
	if gotTopic != stream.TopicCancellation {
		t.Errorf("cancel event published to %q, want %q", gotTopic, stream.TopicCancellation)
	}
}

func TestCancel_transitionFailed(t *testing.T) {
	repo := okRepo()
	repo.transitionFn = func(_ context.Context, _ uuid.UUID, _, _ string) (*model.Notification, error) {
		return nil, db.ErrTransitionFailed
	}
	svc := newSvc(repo, okPublisher(nil))
	_, err := svc.Cancel(context.Background(), uuid.New())
	if !errors.Is(err, db.ErrTransitionFailed) {
		t.Fatalf("expected ErrTransitionFailed, got %v", err)
	}
}

func TestCreateBatch_mixedResults(t *testing.T) {
	svc := newSvc(okRepo(), okPublisher(nil))

	reqs := []service.CreateRequest{
		{Recipient: "+1", Channel: model.ChannelSMS, Content: "valid"},
		{Recipient: "+2", Channel: "invalid", Content: "bad channel"},
		{Recipient: "+3", Channel: model.ChannelEmail, Content: "also valid"},
	}
	_, results, err := svc.CreateBatch(context.Background(), reqs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	accepted := 0
	rejected := 0
	for _, r := range results {
		if r.Status == "accepted" {
			accepted++
		} else {
			rejected++
		}
	}
	if accepted != 2 {
		t.Errorf("accepted = %d, want 2", accepted)
	}
	if rejected != 1 {
		t.Errorf("rejected = %d, want 1", rejected)
	}
}
