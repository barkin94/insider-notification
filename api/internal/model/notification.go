package model

import (
	"encoding/json"
	"time"

	shareddb "github.com/barkin/insider-notification/shared/db"
	"github.com/google/uuid"
)

type Notification struct {
	shareddb.BaseModel `bun:"table:notifications"`
	BatchID            *uuid.UUID      `bun:"batch_id"`
	Recipient          string          `bun:"recipient"`
	Channel            string          `bun:"channel"`
	Content            string          `bun:"content"`
	Priority           string          `bun:"priority"`
	Status             string          `bun:"status"`
	DeliverAfter       *time.Time      `bun:"deliver_after"`
	Attempts           int             `bun:"attempts"`
	MaxAttempts        int             `bun:"max_attempts"`
	Metadata           json.RawMessage `bun:"metadata"`
}
