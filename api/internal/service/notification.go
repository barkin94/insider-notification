package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/barkin/insider-notification/api/internal/domain/notification"
	"github.com/barkin/insider-notification/api/internal/repository"
	sharedErrors "github.com/barkin/insider-notification/shared/errors"
	"github.com/barkin/insider-notification/shared/stream"
)

// BatchResult holds the outcome of one item in a batch create operation.
type BatchResult struct {
	Index  int
	Status string // "accepted" | "rejected"
	ID     *uuid.UUID
	Error  *string
}

// NotificationService defines the business operations for notifications.
type NotificationService interface {
	Create(ctx context.Context, n notification.Notification) (*repository.Notification, error)
	GetByID(ctx context.Context, id uuid.UUID) (*repository.Notification, error)
	List(ctx context.Context, filter repository.ListFilter) ([]*repository.Notification, int, *uuid.UUID, error)
	Cancel(ctx context.Context, id uuid.UUID) (*repository.Notification, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	CreateBatch(ctx context.Context, ns []notification.Notification) (uuid.UUID, []BatchResult, error)
}

type notificationService struct {
	repo      repository.NotificationRepository
	publisher stream.Publisher
}

func NewNotificationService(
	repo repository.NotificationRepository,
	publisher stream.Publisher,
) NotificationService {
	return &notificationService{repo: repo, publisher: publisher}
}

var topicByPriority = map[string]string{
	string(notification.PriorityHigh):   stream.TopicHigh,
	string(notification.PriorityNormal): stream.TopicNormal,
	string(notification.PriorityLow):    stream.TopicLow,
}

func (s *notificationService) Create(ctx context.Context, n notification.Notification) (*repository.Notification, error) {
	entity, err := s.persist(ctx, n, nil)
	if err != nil {
		return nil, err
	}
	if err := s.publish(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (s *notificationService) GetByID(ctx context.Context, id uuid.UUID) (*repository.Notification, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *notificationService) List(ctx context.Context, filter repository.ListFilter) ([]*repository.Notification, int, *uuid.UUID, error) {
	return s.repo.List(ctx, filter)
}

func (s *notificationService) Cancel(ctx context.Context, id uuid.UUID) (*repository.Notification, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, &sharedErrors.NotFoundError{Message: "notification not found"}
		}
		return nil, err
	}

	dn := entity.ToDomain()
	if err := dn.Transition(notification.StatusCancelled); err != nil {
		return nil, &sharedErrors.ConflictError{Message: err.Error()}
	}

	return s.repo.UpdateStatus(ctx, id, string(notification.StatusCancelled))
}

func (s *notificationService) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	dn := entity.ToDomain()
	if err := dn.Transition(notification.Status(status)); err != nil {
		return err
	}

	_, err = s.repo.UpdateStatus(ctx, id, status)
	return err
}

func (s *notificationService) CreateBatch(ctx context.Context, ns []notification.Notification) (uuid.UUID, []BatchResult, error) {
	batchID := uuid.New()
	results := make([]BatchResult, len(ns))

	for i, n := range ns {
		entity, err := s.persist(ctx, n, &batchID)
		if err != nil {
			msg := err.Error()
			results[i] = BatchResult{Index: i, Status: "rejected", Error: &msg}
			continue
		}
		if err := s.publish(ctx, entity); err != nil {
			msg := err.Error()
			results[i] = BatchResult{Index: i, Status: "rejected", Error: &msg}
			continue
		}
		results[i] = BatchResult{Index: i, Status: "accepted", ID: &entity.ID}
	}

	return batchID, results, nil
}

func (s *notificationService) persist(ctx context.Context, n notification.Notification, batchID *uuid.UUID) (*repository.Notification, error) {
	entity, err := repository.Notification{}.From(n, batchID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}
	return entity, nil
}

func (s *notificationService) publish(ctx context.Context, entity *repository.Notification) error {
	if entity.DeliverAfter != nil {
		return nil
	}
	evt := stream.NotificationReadyEvent{}.From(entity)
	if err := s.publisher.Publish(ctx, topicByPriority[entity.Priority], evt); err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	return nil
}
