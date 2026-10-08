package domain

import "time"

// PushSubscription is one browser's permission to be pushed to.
//
// One per browser, not one per person: somebody with a phone and a laptop has
// two, and allowing notifications on one says nothing about the other.
type PushSubscription struct {
	ID     string
	UserID string

	// Endpoint is the URL the browser's push service listens on, and is what
	// identifies the subscription: the same browser re-subscribing produces the
	// same endpoint.
	Endpoint string

	// P256dh and Auth are the browser's keys. The payload is encrypted to them,
	// so the push service that carries it cannot read it.
	P256dh string
	Auth   string

	CreatedAt time.Time
}

// NewPushSubscription returns a subscription ready to be stored.
func NewPushSubscription(id, userID, endpoint, p256dh, auth string, now time.Time) (*PushSubscription, error) {
	if id == "" {
		return nil, ErrEmptySubscriptionID
	}
	if userID == "" {
		return nil, ErrEmptyUserID
	}
	if endpoint == "" || p256dh == "" || auth == "" {
		return nil, ErrInvalidPushSubscription
	}

	return &PushSubscription{
		ID:        id,
		UserID:    userID,
		Endpoint:  endpoint,
		P256dh:    p256dh,
		Auth:      auth,
		CreatedAt: now,
	}, nil
}
