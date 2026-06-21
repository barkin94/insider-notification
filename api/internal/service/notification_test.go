package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/barkin94/insider-notification/api/internal/domain/notification"
	"github.com/barkin94/insider-notification/api/internal/repository"
	"github.com/barkin94/insider-notification/api/internal/service"
	apipub "github.com/barkin94/insider-notification/api/public"
	sharedErrors "github.com/barkin94/insider-notification/shared/genericerrors"
	stream "github.com/barkin94/insider-notification/shared/messaging"
)

// --- mock repo ---

type mockNotifRepo struct {
	createFn       func(ctx context.Context, n *repository.Notification) error
	getByIDFn      func(ctx context.Context, id uuid.UUID) (*repository.Notification, error)
	listFn         func(ctx context.Context, f repository.ListFilter) ([]*repository.Notification, int, *uuid.UUID, error)
	updateStatusFn func(ctx context.Context, id uuid.UUID, status string) (*repository.Notification, error)
}

func (m *mockNotifRepo) Create(ctx context.Context, n *repository.Notification) error {
	return m.createFn(ctx, n)
}
func (m *mockNotifRepo) CreateBatch(ctx context.Context, ns []*repository.Notification) error {
	if m.createFn == nil {
		return nil
	}
	for _, n := range ns {
		if err := m.createFn(ctx, n); err != nil {
			return err
		}
	}
	return nil
}
func (m *mockNotifRepo) GetByID(ctx context.Context, id uuid.UUID) (*repository.Notification, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockNotifRepo) List(ctx context.Context, f repository.ListFilter) ([]*repository.Notification, int, *uuid.UUID, error) {
	return m.listFn(ctx, f)
}
func (m *mockNotifRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (*repository.Notification, error) {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status)
	}
	n := &repository.Notification{Status: status}
	n.ID = id
	return n, nil
}
func (m *mockNotifRepo) FindScheduledDue(_ context.Context) ([]*repository.Notification, error) {
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
		createFn: func(_ context.Context, _ *repository.Notification) error { return nil },
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

func newSvc(repo repository.NotificationRepository, pub stream.Publisher) service.NotificationService {
	return service.NewNotificationService(repo, pub)
}

func validNotification(channel notification.Channel, priority notification.Priority) notification.Notification {
	var n notification.Notification
	n.SetChannel(channel)          //nolint:errcheck,gosec
	n.SetRecipient("+15551234567") //nolint:errcheck,gosec
	n.SetContent("hello")          //nolint:errcheck,gosec
	n.SetPriority(priority)        //nolint:errcheck,gosec
	return n
}

// --- tests ---

func TestCreate_success(t *testing.T) {
	var gotTopic string
	svc := newSvc(okRepo(), okPublisher(&gotTopic))

	n, err := svc.Create(context.Background(), validNotification(notification.ChannelSMS, notification.PriorityHigh))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Status != string(notification.StatusPending) {
		t.Errorf("status = %q, want pending", n.Status)
	}
	if gotTopic != apipub.TopicHigh {
		t.Errorf("published to %q, want %q", gotTopic, apipub.TopicHigh)
	}
}

func TestCreate_publishFailure(t *testing.T) {
	pub := &mockPublisher{publishFn: func(_ context.Context, _ string, _ any) error {
		return errors.New("redis down")
	}}
	svc := newSvc(okRepo(), pub)
	_, err := svc.Create(context.Background(), validNotification(notification.ChannelSMS, notification.PriorityNormal))
	if err == nil {
		t.Fatal("expected error from publisher")
	}
}

func pendingNotif(id uuid.UUID) *repository.Notification {
	n := &repository.Notification{Status: "pending"}
	n.ID = id
	return n
}

func TestCancel_success(t *testing.T) {
	id := uuid.New()
	repo := okRepo()
	repo.getByIDFn = func(_ context.Context, _ uuid.UUID) (*repository.Notification, error) {
		return pendingNotif(id), nil
	}
	svc := newSvc(repo, okPublisher(nil))
	n, err := svc.Cancel(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Status != string(notification.StatusCancelled) {
		t.Errorf("status = %q, want cancelled", n.Status)
	}
}

func TestCancel_notFound(t *testing.T) {
	repo := okRepo()
	repo.getByIDFn = func(_ context.Context, _ uuid.UUID) (*repository.Notification, error) {
		return nil, repository.ErrNotFound
	}
	svc := newSvc(repo, okPublisher(nil))
	_, err := svc.Cancel(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var notFoundErr *sharedErrors.NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestCancel_invalidTransition(t *testing.T) {
	id := uuid.New()
	repo := okRepo()
	repo.getByIDFn = func(_ context.Context, _ uuid.UUID) (*repository.Notification, error) {
		n := &repository.Notification{Status: "delivered"}
		n.ID = id
		return n, nil
	}
	svc := newSvc(repo, okPublisher(nil))
	_, err := svc.Cancel(context.Background(), id)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var conflictErr *sharedErrors.ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}
