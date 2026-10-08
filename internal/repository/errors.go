package repository

import "errors"

// Lookup failures of the storage layer.
var (
	ErrEventNotFound          = errors.New("event not found")
	ErrEventExists            = errors.New("an event with that id already exists")
	ErrEventInUse             = errors.New("the event has seats that are held or sold")
	ErrSeatNotFound           = errors.New("seat not found")
	ErrHoldNotFound           = errors.New("hold not found")
	ErrConcurrentUpdate       = errors.New("the seat was changed by someone else too many times")
	ErrIdempotencyKeyNotFound = errors.New("idempotency key not found")
	ErrUserNotFound           = errors.New("user not found")
	ErrEmailTaken             = errors.New("that email address is already registered")
	ErrWaitingListEmpty       = errors.New("nobody is waiting")
	ErrNotWaiting             = errors.New("the user is not on the waiting list")
	ErrDeliveryNotFound       = errors.New("delivery not found")
	ErrNotificationNotFound   = errors.New("notification not found")
)
