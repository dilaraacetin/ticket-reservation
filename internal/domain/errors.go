package domain

import "errors"

// Sentinel errors returned by the domain layer, compared with errors.Is so that
// outer layers stay free to wrap them with context. The set is this fine
// grained because stage 3 maps each one onto a different HTTP status code.
var (
	ErrSeatNotAvailable = errors.New("seat is not available")

	ErrSeatNotHeld = errors.New("seat is not held")

	ErrHoldExpired = errors.New("hold has expired")

	ErrNotHoldOwner = errors.New("hold belongs to another user")

	ErrEmptyUserID = errors.New("user id must not be empty")

	ErrEmptyHoldID = errors.New("hold id must not be empty")

	ErrInvalidHoldDuration = errors.New("hold duration must be positive")

	ErrUnknownSeatStatus = errors.New("unknown seat status")

	ErrInvalidEmail = errors.New("the email address is not valid")

	ErrWeakPassword = errors.New("the password does not meet the requirements")
)
