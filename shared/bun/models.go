package bun

import (
	"time"

	"github.com/google/uuid"
)

type IDModel struct {
	ID uuid.UUID `bun:",pk,type:uuid"`
}

func NewIDModel() (IDModel, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return IDModel{}, err
	}
	return IDModel{ID: id}, nil
}

type TimestampModel struct {
	CreatedAt time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp"`
}

type TraceMetadataModel struct {
	TraceMetadata map[string]any `bun:"trace_metadata,type:jsonb,nullzero"`
}
