package service

import (
	"context"
	"errors"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// ErrNotYourSubscription means the caller asked to forget a browser that is not
// theirs.
var ErrNotYourSubscription = errors.New("that subscription belongs to another account")

// PushService records which browsers have agreed to be pushed to.
type PushService struct {
	subscriptions repository.PushRepository
	publicKey     string
	clock         Clock
	newID         func() string
}

func NewPushService(
	subscriptions repository.PushRepository,
	publicKey string,
	clock Clock,
	newID func() string,
) *PushService {
	return &PushService{subscriptions: subscriptions, publicKey: publicKey, clock: clock, newID: newID}
}

// PublicKey is what a browser needs before it can subscribe.
func (s *PushService) PublicKey() string {
	return s.publicKey
}

// Subscribe stores a browser's permission.
func (s *PushService) Subscribe(ctx context.Context, userID, endpoint, p256dh, auth string) error {
	subscription, err := domain.NewPushSubscription(s.newID(), userID, endpoint, p256dh, auth, s.clock.Now())
	if err != nil {
		return err
	}

	return s.subscriptions.Subscribe(ctx, subscription)
}

// Unsubscribe forgets one browser, and only the caller's own.
//
// The endpoint alone would be enough to delete by, which is exactly why it is
// checked: endpoints are long and unguessable but they are not secrets, and
// without this anybody holding one could switch somebody else's notifications
// off.
func (s *PushService) Unsubscribe(ctx context.Context, userID, endpoint string) error {
	if endpoint == "" {
		return domain.ErrInvalidPushSubscription
	}

	mine, err := s.subscriptions.ListForUser(ctx, userID)
	if err != nil {
		return err
	}

	for _, subscription := range mine {
		if subscription.Endpoint == endpoint {
			return s.subscriptions.Unsubscribe(ctx, endpoint)
		}
	}

	return ErrNotYourSubscription
}
