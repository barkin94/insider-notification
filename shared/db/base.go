package db

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// BaseModel is embedded by all table models. It provides bun ORM metadata
// and the common fields present on every table: id, created_at, updated_at.
type BaseModel struct {
	bun.BaseModel
	ID        uuid.UUID `bun:",pk,type:uuid"`
	CreatedAt time.Time `bun:",nullzero,noinsert,default:current_timestamp"`
	UpdatedAt time.Time `bun:",nullzero,default:current_timestamp"`
}
