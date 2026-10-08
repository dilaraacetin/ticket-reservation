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

	ErrEmptyTicketCode = errors.New("ticket code must not be empty")

	ErrEmptyEventID = errors.New("event id must not be empty")

	ErrEmptyEntryID = errors.New("waiting list entry id must not be empty")

	ErrInvalidHoldDuration = errors.New("hold duration must be positive")

	ErrUnknownSeatStatus = errors.New("unknown seat status")

	ErrUnknownRole = errors.New("unknown role")

	ErrUnknownCategory = errors.New("unknown category")

	ErrUnknownNotificationKind = errors.New("unknown notification kind")

	ErrUnknownDeliveryChannel = errors.New("unknown delivery channel")

	ErrEmptySubscriptionID = errors.New("subscription id must not be empty")

	ErrEmptyVerificationToken = errors.New("verification token must not be empty")

	ErrVerificationNotUsable = errors.New("that verification link is no longer valid")

	ErrEmailNotVerified = errors.New("the email address has not been verified")

	ErrInvalidPushSubscription = errors.New("the push subscription is missing its endpoint or keys")

	ErrEmptyNotificationID = errors.New("notification id must not be empty")

	ErrNotPermitted = errors.New("this account is not allowed to do that")

	ErrInvalidEvent = errors.New("the event is not valid")

	ErrEventCancelled = errors.New("the event has been cancelled")

	ErrEventAlreadyCancelled = errors.New("the event is already cancelled")

	ErrInvalidSeatMap = errors.New("the seat map is not valid")

	ErrInvalidEmail = errors.New("the email address is not valid")

	ErrWeakPassword = errors.New("the password does not meet the requirements")
)
