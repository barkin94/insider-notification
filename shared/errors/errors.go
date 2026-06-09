package errors

// DomainError is a typed validation or state error produced by the domain layer.
type DomainError struct {
	Code    string
	Message string
}

var _ error = (*DomainError)(nil) // ensure it implements error

func (e DomainError) Error() string { return e.Message }

// Is compares by Code so errors.Is works across independently constructed values.
func (e DomainError) Is(target error) bool {
	t, ok := target.(DomainError)
	return ok && t.Code == e.Code
}

// NotFoundError signals that a requested resource does not exist (HTTP 404).
type NotFoundError struct{ Message string }

func (e *NotFoundError) Error() string { return e.Message }

// ConflictError signals a state conflict, such as an invalid status transition (HTTP 409).
type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }
