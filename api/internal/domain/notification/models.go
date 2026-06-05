package notification

import (
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"time"
)

type Channel string
type Priority string
type Status string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"

	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"

	StatusPending   Status = "pending"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// E.164: + followed by 7–15 digits, leading digit non-zero.
var phoneRE = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

var validTransitions = map[Status][]Status{
	StatusPending:   {StatusDelivered, StatusFailed, StatusCancelled},
	StatusFailed:    {StatusPending}, // retry
	StatusDelivered: {},              // terminal
	StatusCancelled: {},              // terminal
}

type Notification struct {
	channel      Channel
	recipient    string
	content      string
	priority     Priority
	status       Status
	deliverAfter *time.Time
}

func (n *Notification) SetChannel(ch Channel) error {
	switch ch {
	case ChannelSMS, ChannelEmail, ChannelPush:
		n.channel = ch
		return nil
	default:
		return ErrInvalidChannel()
	}
}

// SetRecipient must be called after SetChannel; format is validated per channel.
func (n *Notification) SetRecipient(r string) error {
	if r == "" {
		return ErrRecipientRequired()
	}
	switch n.channel {
	case ChannelEmail:
		if _, err := mail.ParseAddress(r); err != nil {
			return ErrInvalidEmail()
		}
	case ChannelSMS:
		if !phoneRE.MatchString(r) {
			return ErrInvalidPhone()
		}
	case ChannelPush:
		// no format restriction beyond non-empty
	default:
		return ErrChannelNotSet()
	}
	n.recipient = r
	return nil
}

func (n *Notification) SetContent(s string) error {
	if s == "" {
		return ErrContentRequired()
	}
	n.content = s
	return nil
}

func (n *Notification) SetPriority(p Priority) error {
	switch p {
	case PriorityHigh, PriorityNormal, PriorityLow:
		n.priority = p
		return nil
	default:
		return ErrInvalidPriority()
	}
}

func (n *Notification) SetDeliverAfter(t *time.Time) {
	n.deliverAfter = t
}

func (n *Notification) Transition(to Status) error {
	allowed, ok := validTransitions[n.status]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownStatus(), n.status)
	}
	if slices.Contains(allowed, to) {
		n.status = to
		return nil
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidTransition(), n.status, to)
}

func (n Notification) GetChannel() Channel         { return n.channel }
func (n Notification) GetRecipient() string        { return n.recipient }
func (n Notification) GetContent() string          { return n.content }
func (n Notification) GetPriority() Priority       { return n.priority }
func (n Notification) GetStatus() Status           { return n.status }
func (n Notification) GetDeliverAfter() *time.Time { return n.deliverAfter }
