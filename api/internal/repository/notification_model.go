package repository

import (
	"time"

	"github.com/google/uuid"

	"github.com/barkin94/insider-notification/api/internal/domain/notification"
	sharedbun "github.com/barkin94/insider-notification/shared/bun"
)

type Notification struct {
	sharedbun.BaseModel `bun:"table:notifications"`
	BatchID             *uuid.UUID `bun:"batch_id"`
	Recipient           string     `bun:"recipient"`
	Channel             string     `bun:"channel"`
	Content             string     `bun:"content"`
	Priority            string     `bun:"priority"`
	Status              string     `bun:"status"`
	DeliverAfter        *time.Time `bun:"deliver_after"`
	MaxAttempts         int        `bun:"max_attempts"`
}

func (Notification) From(n notification.Notification, batchID *uuid.UUID) (*Notification, error) {
	base, err := sharedbun.NewBaseModel()
	if err != nil {
		return nil, err
	}
	return &Notification{
		BaseModel:    base,
		BatchID:      batchID,
		Recipient:    n.GetRecipient(),
		Channel:      string(n.GetChannel()),
		Content:      n.GetContent(),
		Priority:     string(n.GetPriority()),
		Status:       string(notification.StatusPending),
		MaxAttempts:  n.GetMaxAttempts(),
		DeliverAfter: n.GetDeliverAfter(),
	}, nil
}

func (n *Notification) ToDomain() *notification.Notification {
	return notification.New(
		notification.Channel(n.Channel),
		n.Recipient,
		n.Content,
		notification.Priority(n.Priority),
		notification.Status(n.Status),
		n.DeliverAfter,
		n.MaxAttempts,
	)
}

func (n *Notification) GetID() string        { return n.ID.String() }
func (n *Notification) GetChannel() string   { return n.Channel }
func (n *Notification) GetRecipient() string { return n.Recipient }
func (n *Notification) GetContent() string   { return n.Content }
func (n *Notification) GetPriority() string  { return n.Priority }
func (n *Notification) GetMaxAttempts() int  { return n.MaxAttempts }
