package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/google/uuid"
)

// StreamPublisher is the port for publishing events to the message stream.
type StreamPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

// CreateRequest carries validated input for creating a notification.
type CreateRequest struct {
	Recipient    string
	Channel      string
	Content      string
	Priority     string
	Metadata     json.RawMessage
	DeliverAfter *time.Time
}

// BatchResult holds the outcome of one item in a batch create request.
type BatchResult struct {
	Index  int
	Status string // "accepted" | "rejected"
	ID     *uuid.UUID
	Error  *string
}

// NotificationService defines the business operations for notifications.
type NotificationService interface {
	Create(ctx context.Context, req CreateRequest) (*model.Notification, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, []*model.DeliveryAttempt, error)
	List(ctx context.Context, filter db.ListFilter) ([]*model.Notification, int, error)
	Cancel(ctx context.Context, id uuid.UUID) (*model.Notification, error)
	CreateBatch(ctx context.Context, reqs []CreateRequest) (uuid.UUID, []BatchResult, error)
}

type notificationService struct {
	repo      db.NotificationRepository
	attempts  db.DeliveryAttemptRepository
	publisher StreamPublisher
}

func NewNotificationService(
	repo db.NotificationRepository,
	attempts db.DeliveryAttemptRepository,
	publisher StreamPublisher,
) NotificationService {
	return &notificationService{repo: repo, attempts: attempts, publisher: publisher}
}

var contentLimits = map[string]int{
	model.ChannelSMS:   1600,
	model.ChannelEmail: 100_000,
	model.ChannelPush:  4096,
}

var validChannels = map[string]bool{
	model.ChannelSMS:   true,
	model.ChannelEmail: true,
	model.ChannelPush:  true,
}

var validPriorities = map[string]bool{
	model.PriorityHigh:   true,
	model.PriorityNormal: true,
	model.PriorityLow:    true,
}

var topicByPriority = map[string]string{
	model.PriorityHigh:   stream.TopicHigh,
	model.PriorityNormal: stream.TopicNormal,
	model.PriorityLow:    stream.TopicLow,
}

// ValidationError carries a human-readable field-level validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
}

func validate(req CreateRequest) error {
	if req.Recipient == "" || len(req.Recipient) > 255 {
		return &ValidationError{Field: "recipient", Message: "required, max 255 chars"}
	}
	if !validChannels[req.Channel] {
		return &ValidationError{Field: "channel", Message: "must be one of: sms, email, push"}
	}
	if req.Content == "" {
		return &ValidationError{Field: "content", Message: "required"}
	}
	if limit, ok := contentLimits[req.Channel]; ok && len(req.Content) > limit {
		return &ValidationError{Field: "content", Message: fmt.Sprintf("exceeds %d char limit for %s", limit, req.Channel)}
	}
	if req.Priority != "" && !validPriorities[req.Priority] {
		return &ValidationError{Field: "priority", Message: "must be one of: high, normal, low"}
	}
	return nil
}

func (s *notificationService) Create(ctx context.Context, req CreateRequest) (*model.Notification, error) {
	if err := validate(req); err != nil {
		return nil, err
	}

	priority := req.Priority
	if priority == "" {
		priority = model.PriorityNormal
	}

	metadata := req.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage("{}")
	}

	now := time.Now().UTC()
	n := &model.Notification{
		ID:           uuid.New(),
		Recipient:    req.Recipient,
		Channel:      req.Channel,
		Content:      req.Content,
		Priority:     priority,
		Status:       model.StatusPending,
		MaxAttempts:  4,
		Metadata:     metadata,
		DeliverAfter: req.DeliverAfter,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}

	deliverAfter := ""
	if n.DeliverAfter != nil {
		deliverAfter = n.DeliverAfter.Format(time.RFC3339)
	}

	evt := stream.NotificationCreatedEvent{
		NotificationID: n.ID.String(),
		Channel:        n.Channel,
		Recipient:      n.Recipient,
		Content:        n.Content,
		Priority:       n.Priority,
		AttemptNumber:  1,
		MaxAttempts:    n.MaxAttempts,
		DeliverAfter:   deliverAfter,
		Metadata:       string(n.Metadata),
	}

	topic := topicByPriority[priority]
	if err := s.publisher.Publish(ctx, topic, evt); err != nil {
		return nil, fmt.Errorf("publish event: %w", err)
	}

	return n, nil
}

func (s *notificationService) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, []*model.DeliveryAttempt, error) {
	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	attempts, err := s.attempts.ListByNotificationID(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("list attempts: %w", err)
	}
	return n, attempts, nil
}

func (s *notificationService) List(ctx context.Context, filter db.ListFilter) ([]*model.Notification, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *notificationService) Cancel(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	n, err := s.repo.Transition(ctx, id, model.StatusPending, model.StatusCancelled)
	if err != nil {
		return nil, err
	}
	evt := stream.NotificationCancelledEvent{NotificationID: id.String()}
	_ = s.publisher.Publish(ctx, stream.TopicCancellation, evt)
	return n, nil
}

func (s *notificationService) CreateBatch(ctx context.Context, reqs []CreateRequest) (uuid.UUID, []BatchResult, error) {
	batchID := uuid.New()
	results := make([]BatchResult, len(reqs))

	for i, req := range reqs {
		if err := validate(req); err != nil {
			msg := err.Error()
			results[i] = BatchResult{Index: i, Status: "rejected", Error: &msg}
			continue
		}

		n, err := s.createWithBatchID(ctx, req, batchID)
		if err != nil {
			msg := err.Error()
			results[i] = BatchResult{Index: i, Status: "rejected", Error: &msg}
			continue
		}
		results[i] = BatchResult{Index: i, Status: "accepted", ID: &n.ID}
	}

	return batchID, results, nil
}

func (s *notificationService) createWithBatchID(ctx context.Context, req CreateRequest, batchID uuid.UUID) (*model.Notification, error) {
	priority := req.Priority
	if priority == "" {
		priority = model.PriorityNormal
	}

	metadata := req.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage("{}")
	}

	now := time.Now().UTC()
	n := &model.Notification{
		ID:           uuid.New(),
		BatchID:      &batchID,
		Recipient:    req.Recipient,
		Channel:      req.Channel,
		Content:      req.Content,
		Priority:     priority,
		Status:       model.StatusPending,
		MaxAttempts:  4,
		Metadata:     metadata,
		DeliverAfter: req.DeliverAfter,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}

	deliverAfter := ""
	if n.DeliverAfter != nil {
		deliverAfter = n.DeliverAfter.Format(time.RFC3339)
	}

	evt := stream.NotificationCreatedEvent{
		NotificationID: n.ID.String(),
		Channel:        n.Channel,
		Recipient:      n.Recipient,
		Content:        n.Content,
		Priority:       n.Priority,
		AttemptNumber:  1,
		MaxAttempts:    n.MaxAttempts,
		DeliverAfter:   deliverAfter,
		Metadata:       string(n.Metadata),
	}

	topic := topicByPriority[priority]
	if err := s.publisher.Publish(ctx, topic, evt); err != nil {
		return nil, fmt.Errorf("publish event: %w", err)
	}

	return n, nil
}

// IsValidationError reports whether err is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
