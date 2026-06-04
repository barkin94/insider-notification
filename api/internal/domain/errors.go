package domain

import sharedErrors "github.com/barkin/insider-notification/shared/errors"

// DomainError is an alias so callers reference domain.DomainError
// while the type itself lives in shared/errors.
type DomainError = sharedErrors.DomainError

func ErrInvalidChannel() DomainError {
	return DomainError{Code: "INVALID_CHANNEL", Message: "must be one of sms, email, push"}
}

func ErrRecipientRequired() DomainError {
	return DomainError{Code: "RECIPIENT_REQUIRED", Message: "required"}
}

func ErrInvalidEmail() DomainError {
	return DomainError{Code: "INVALID_EMAIL", Message: "must be a valid email address"}
}

func ErrInvalidPhone() DomainError {
	return DomainError{Code: "INVALID_PHONE", Message: "must be a valid E.164 phone number (+<7-15 digits>)"}
}

func ErrChannelNotSet() DomainError {
	return DomainError{Code: "CHANNEL_NOT_SET", Message: "channel must be set before recipient"}
}

func ErrContentRequired() DomainError {
	return DomainError{Code: "CONTENT_REQUIRED", Message: "required"}
}

func ErrInvalidPriority() DomainError {
	return DomainError{Code: "INVALID_PRIORITY", Message: "must be one of high, normal, low"}
}

func ErrInvalidDeliverAfter() DomainError {
	return DomainError{Code: "INVALID_DELIVER_AFTER", Message: "must be ISO 8601 format"}
}

func ErrUnknownStatus() DomainError {
	return DomainError{Code: "UNKNOWN_STATUS", Message: "unknown status"}
}

func ErrInvalidTransition() DomainError {
	return DomainError{Code: "INVALID_TRANSITION", Message: "invalid status transition"}
}
