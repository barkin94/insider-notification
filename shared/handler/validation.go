package handler

import (
	"strings"
)

// FieldError describes a single field-level validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError is a domain error carrying one or more field-level failures.
// errHandler renders it as 422 with structured details.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Field + ": " + f.Message
	}
	return "validation: " + strings.Join(parts, "; ")
}

// NewValidationError is a convenience constructor for a single field failure.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: message}}}
}
