package repository

import "errors"

// Lookup failures of the storage layer.
var (
	ErrSeatNotFound = errors.New("seat not found")
	ErrHoldNotFound = errors.New("hold not found")
)
