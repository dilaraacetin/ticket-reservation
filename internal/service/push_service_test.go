package service

import (
	"errors"
	"testing"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

func newPushSetup(t *testing.T) (*PushService, *repository.MemoryPushRepository) {
	t.Helper()

	store := repository.NewMemoryPushRepository()

	return NewPushService(store, "BPublicKey", newFakeClock(testTime()), NewRandomID), store
}

const (
	minePhone    = "https://push.example.com/mine"
	theirsLaptop = "https://push.example.com/theirs"
)

func TestPush_SubscribeAndUnsubscribe(t *testing.T) {
	push, store := newPushSetup(t)

	err := push.Subscribe(t.Context(), testUser, minePhone, "BKey", "c2VjcmV0")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	mine, err := store.ListForUser(t.Context(), testUser)
	if err != nil {
		t.Fatalf("ListForUser() error = %v", err)
	}
	if len(mine) != 1 || mine[0].Endpoint != minePhone {
		t.Fatalf("stored %v, want one for %s", mine, minePhone)
	}

	if err := push.Unsubscribe(t.Context(), testUser, minePhone); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}

	if mine, _ := store.ListForUser(t.Context(), testUser); len(mine) != 0 {
		t.Errorf("still subscribed after unsubscribing: %v", mine)
	}
}

// An endpoint is long and unguessable but it is not a secret. Without the
// ownership check anybody holding one could switch somebody else's notifications
// off.
func TestPush_CannotUnsubscribeSomebodyElse(t *testing.T) {
	push, store := newPushSetup(t)

	if err := push.Subscribe(t.Context(), otherUser, theirsLaptop, "BKey", "c2VjcmV0"); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	err := push.Unsubscribe(t.Context(), testUser, theirsLaptop)
	if !errors.Is(err, ErrNotYourSubscription) {
		t.Fatalf("Unsubscribe() error = %v, want %v", err, ErrNotYourSubscription)
	}

	// And it is still there.
	theirs, _ := store.ListForUser(t.Context(), otherUser)
	if len(theirs) != 1 {
		t.Error("somebody else's subscription was removed")
	}
}

func TestPush_RefusesAnIncompleteSubscription(t *testing.T) {
	push, _ := newPushSetup(t)

	tests := []struct{ name, endpoint, p256dh, auth string }{
		{"no endpoint", "", "BKey", "c2VjcmV0"},
		{"no key", minePhone, "", "c2VjcmV0"},
		{"no auth", minePhone, "BKey", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := push.Subscribe(t.Context(), testUser, tt.endpoint, tt.p256dh, tt.auth)
			if !errors.Is(err, domain.ErrInvalidPushSubscription) {
				t.Errorf("Subscribe() error = %v, want %v", err, domain.ErrInvalidPushSubscription)
			}
		})
	}

	if err := push.Unsubscribe(t.Context(), testUser, ""); !errors.Is(err, domain.ErrInvalidPushSubscription) {
		t.Errorf("Unsubscribe() with no endpoint = %v, want %v", err, domain.ErrInvalidPushSubscription)
	}
}
