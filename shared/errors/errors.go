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
