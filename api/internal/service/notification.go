package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/barkin94/insider-notification/api/internal/domain/notification"
	"github.com/barkin94/insider-notification/api/internal/repository"
	apipub "github.com/barkin94/insider-notification/api/public"
	sharedErrors "github.com/barkin94/insider-notification/shared/genericerrors"
	stream "github.com/barkin94/insider-notification/shared/messaging"
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
	string(notification.PriorityHigh):   apipub.TopicHigh,
	string(notification.PriorityNormal): apipub.TopicNormal,
	string(notification.PriorityLow):    apipub.TopicLow,
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

	// Convert all to entities and validate
	entities := make([]*repository.Notification, len(ns))
	for i, n := range ns {
		entity, err := repository.Notification{}.From(n, &batchID)
		if err != nil {
			msg := err.Error()
			results[i] = BatchResult{Index: i, Status: "rejected", Error: &msg}
			continue
		}
		entities[i] = entity
		results[i] = BatchResult{Index: i, Status: "accepted", ID: &entity.ID}
	}

	// Collect valid entities and persist in batch
	acceptedEntities := make([]*repository.Notification, 0, len(ns))
	acceptedIndices := make([]int, 0, len(ns))
	for i, entity := range entities {
		if entity != nil {
			acceptedEntities = append(acceptedEntities, entity)
			acceptedIndices = append(acceptedIndices, i)
		}
	}

	if len(acceptedEntities) > 0 {
		if err := s.repo.CreateBatch(ctx, acceptedEntities); err != nil {
			msg := fmt.Sprintf("create notifications: %v", err)
			for _, i := range acceptedIndices {
				results[i].Status = "rejected"
				results[i].Error = &msg
				results[i].ID = nil
			}
			acceptedEntities = nil
		}
	}

	// Separate scheduled and immediate notifications
	scheduled := make([]apipub.ScheduledNotificationItem, 0)
	immediate := make([]*repository.Notification, 0)

	for _, entity := range acceptedEntities {
		if entity.DeliverAfter != nil {
			scheduled = append(scheduled, apipub.ScheduledNotificationItem{
				NotificationID: entity.ID.String(),
				ScheduledAt:    *entity.DeliverAfter,
			})
		} else {
			immediate = append(immediate, entity)
		}
	}

	// Publish scheduled notifications as a batch
	if len(scheduled) > 0 {
		evt := apipub.NotificationsScheduledEvent{Notifications: scheduled}
		if err := s.publisher.Publish(ctx, apipub.TopicNotificationScheduled, evt); err != nil {
			return batchID, results, fmt.Errorf("publish scheduled events: %w", err)
		}
	}

	// Publish immediate notifications
	for _, entity := range immediate {
		evt := apipub.NotificationReadyEvent{}.From(entity)
		if err := s.publisher.Publish(ctx, topicByPriority[entity.Priority], evt); err != nil {
			return batchID, results, fmt.Errorf("publish ready event: %w", err)
		}
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
		// Scheduled notification: publish to delivery scheduler service
		evt := apipub.NotificationsScheduledEvent{
			Notifications: []apipub.ScheduledNotificationItem{
				{
					NotificationID: entity.ID.String(),
					ScheduledAt:    *entity.DeliverAfter,
				},
			},
		}
		if err := s.publisher.Publish(ctx, apipub.TopicNotificationScheduled, evt); err != nil {
			return fmt.Errorf("publish scheduled event: %w", err)
		}
		return nil
	}
	evt := apipub.NotificationReadyEvent{}.From(entity)
	if err := s.publisher.Publish(ctx, topicByPriority[entity.Priority], evt); err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	return nil
}
