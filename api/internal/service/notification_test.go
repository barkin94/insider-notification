package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/api/internal/db/entities"
	"github.com/barkin/insider-notification/api/internal/db/repos"
	"github.com/barkin/insider-notification/api/internal/domain"
	"github.com/barkin/insider-notification/api/internal/service"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
)

// --- mock repo ---

type mockNotifRepo struct {
	createFn       func(ctx context.Context, n *entities.Notification) error
	getByIDFn      func(ctx context.Context, id uuid.UUID) (*entities.Notification, error)
	listFn         func(ctx context.Context, f repos.ListFilter) ([]*entities.Notification, int, *uuid.UUID, error)
	transitionFn   func(ctx context.Context, id uuid.UUID, from, to string) (*entities.Notification, error)
	incrFn         func(ctx context.Context, id uuid.UUID) error
	updateStatusFn func(ctx context.Context, id uuid.UUID, status string) error
}

func (m *mockNotifRepo) Create(ctx context.Context, n *entities.Notification) error {
	return m.createFn(ctx, n)
}
func (m *mockNotifRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.Notification, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockNotifRepo) List(ctx context.Context, f repos.ListFilter) ([]*entities.Notification, int, *uuid.UUID, error) {
	return m.listFn(ctx, f)
}
func (m *mockNotifRepo) Transition(ctx context.Context, id uuid.UUID, from, to string) (*entities.Notification, error) {
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
func (m *mockNotifRepo) FindScheduledDue(_ context.Context) ([]*entities.Notification, error) {
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
		createFn: func(_ context.Context, _ *entities.Notification) error { return nil },
		transitionFn: func(_ context.Context, id uuid.UUID, _, to string) (*entities.Notification, error) {
			n := &entities.Notification{Status: to}
			n.ID = id
			return n, nil
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

func newSvc(repo repos.NotificationRepository, pub stream.Publisher) service.NotificationService {
	return service.NewNotificationService(repo, pub)
}

func validNotification(channel domain.Channel, priority domain.Priority) domain.Notification {
	var n domain.Notification
	n.SetChannel(channel)         //nolint:errcheck
	n.SetRecipient("+15551234567") //nolint:errcheck
	n.SetContent("hello")         //nolint:errcheck
	n.SetPriority(priority)       //nolint:errcheck
	return n
}

// --- tests ---

func TestCreate_success(t *testing.T) {
	var gotTopic string
	svc := newSvc(okRepo(), okPublisher(&gotTopic))

	n, err := svc.Create(context.Background(), validNotification(domain.ChannelSMS, domain.PriorityHigh))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Status != string(domain.StatusPending) {
		t.Errorf("status = %q, want pending", n.Status)
	}
	if gotTopic != stream.TopicHigh {
		t.Errorf("published to %q, want %q", gotTopic, stream.TopicHigh)
	}
}

func TestCreate_publishFailure(t *testing.T) {
	pub := &mockPublisher{publishFn: func(_ context.Context, _ string, _ any) error {
		return errors.New("redis down")
	}}
	svc := newSvc(okRepo(), pub)
	_, err := svc.Create(context.Background(), validNotification(domain.ChannelSMS, domain.PriorityNormal))
	if err == nil {
		t.Fatal("expected error from publisher")
	}
}

func TestCancel_success(t *testing.T) {
	svc := newSvc(okRepo(), okPublisher(nil))
	n, err := svc.Cancel(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Status != string(domain.StatusCancelled) {
		t.Errorf("status = %q, want cancelled", n.Status)
	}
}

func TestCancel_transitionFailed(t *testing.T) {
	repo := okRepo()
	repo.transitionFn = func(_ context.Context, _ uuid.UUID, _, _ string) (*entities.Notification, error) {
		return nil, db.ErrTransitionFailed
	}
	svc := newSvc(repo, okPublisher(nil))
	_, err := svc.Cancel(context.Background(), uuid.New())
	if !errors.Is(err, db.ErrTransitionFailed) {
		t.Fatalf("expected ErrTransitionFailed, got %v", err)
	}
}
