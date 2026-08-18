package repository

import "errors"

// Lookup failures of the storage layer.
var (
	ErrEventNotFound          = errors.New("event not found")
	ErrSeatNotFound           = errors.New("seat not found")
	ErrHoldNotFound           = errors.New("hold not found")
	ErrConcurrentUpdate       = errors.New("the seat was changed by someone else too many times")
	ErrIdempotencyKeyNotFound = errors.New("idempotency key not found")
)
