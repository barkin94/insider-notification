package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/barkin/insider-notification/api/internal/db/entities"
	"github.com/barkin/insider-notification/api/internal/db/repos"
	"github.com/barkin/insider-notification/api/internal/domain"
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
	Create(ctx context.Context, n domain.Notification) (*entities.Notification, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entities.Notification, error)
	List(ctx context.Context, filter repos.ListFilter) ([]*entities.Notification, int, *uuid.UUID, error)
	Cancel(ctx context.Context, id uuid.UUID) (*entities.Notification, error)
	CreateBatch(ctx context.Context, ns []domain.Notification) (uuid.UUID, []BatchResult, error)
}

type notificationService struct {
	repo      repos.NotificationRepository
	publisher stream.Publisher
}

func NewNotificationService(
	repo repos.NotificationRepository,
	publisher stream.Publisher,
) NotificationService {
	return &notificationService{repo: repo, publisher: publisher}
}

var topicByPriority = map[string]string{
	string(domain.PriorityHigh):   stream.TopicHigh,
	string(domain.PriorityNormal): stream.TopicNormal,
	string(domain.PriorityLow):    stream.TopicLow,
}

func (s *notificationService) Create(ctx context.Context, n domain.Notification) (*entities.Notification, error) {
	entity, err := s.persist(ctx, n, nil)
	if err != nil {
		return nil, err
	}
	if err := s.publish(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (s *notificationService) GetByID(ctx context.Context, id uuid.UUID) (*entities.Notification, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *notificationService) List(ctx context.Context, filter repos.ListFilter) ([]*entities.Notification, int, *uuid.UUID, error) {
	return s.repo.List(ctx, filter)
}

func (s *notificationService) Cancel(ctx context.Context, id uuid.UUID) (*entities.Notification, error) {
	return s.repo.Transition(ctx, id, string(domain.StatusPending), string(domain.StatusCancelled))
}

func (s *notificationService) CreateBatch(ctx context.Context, ns []domain.Notification) (uuid.UUID, []BatchResult, error) {
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

func (s *notificationService) persist(ctx context.Context, n domain.Notification, batchID *uuid.UUID) (*entities.Notification, error) {
	entity, err := entities.Notification{}.From(n, batchID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}
	return entity, nil
}

func (s *notificationService) publish(ctx context.Context, entity *entities.Notification) error {
	if entity.DeliverAfter != nil {
		return nil
	}
	evt := stream.NotificationReadyEvent{}.From(entity)
	if err := s.publisher.Publish(ctx, topicByPriority[entity.Priority], evt); err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	return nil
}
