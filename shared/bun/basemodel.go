package bun

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
	CreatedAt time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp"`
}

func NewBaseModel() (BaseModel, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return BaseModel{}, err
	}
	return BaseModel{ID: id}, nil
}

// func (m *BaseModel) BeforeUpdate(_ context.Context, _ *bun.UpdateQuery) error {
// 	m.UpdatedAt = time.Now().UTC()
// 	return nil
// }
