package repository

import (
	"testing"

	"ticket-reservation/internal/domain"
)

type pushRepositoryFactory func(t *testing.T) PushRepository

func newMemoryPush(_ *testing.T) PushRepository {
	return NewMemoryPushRepository()
}

func newPostgresPush(t *testing.T) PushRepository {
	t.Helper()

	pool := newTestPool(t)
	if _, err := pool.Exec(t.Context(), "truncate table push_subscriptions"); err != nil {
		t.Fatalf("resetting push_subscriptions failed: %v", err)
	}

	return NewPostgresPushRepository(pool)
}

func testSubscription(t *testing.T, id, userID, endpoint, p256dh string) *domain.PushSubscription {
	t.Helper()

	subscription, err := domain.NewPushSubscription(
		id, userID, endpoint, p256dh, "c2VjcmV0LWF1dGgtdG9rZW4", testTime())
	if err != nil {
		t.Fatalf("NewPushSubscription() error = %v", err)
	}

	return subscription
}

func TestPushRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo pushRepositoryFactory
	}{
		{"memory", newMemoryPush},
		{"postgres", newPostgresPush},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runPushRepositoryContract(t, implementation.newRepo)
		})
	}
}

func runPushRepositoryContract(t *testing.T, newRepo pushRepositoryFactory) {
	t.Helper()

	const (
		phone  = "https://push.example.com/phone-endpoint"
		laptop = "https://push.example.com/laptop-endpoint"
	)

	t.Run("somebody who has allowed nothing has nothing", func(t *testing.T) {
		repo := newRepo(t)

		found, err := repo.ListForUser(t.Context(), "dilara")
		if err != nil {
			t.Fatalf("ListForUser() error = %v", err)
		}
		if found == nil {
			t.Error("got nil, want an empty slice")
		}
		if len(found) != 0 {
			t.Errorf("got %d subscriptions, want none", len(found))
		}
	})

	// One per browser, not one per person: allowing notifications on a phone
	// says nothing about a laptop.
	t.Run("a person can have one per browser", func(t *testing.T) {
		repo := newRepo(t)

		for _, subscription := range []*domain.PushSubscription{
			testSubscription(t, "s-phone", "dilara", phone, "BPhoneKey"),
			testSubscription(t, "s-laptop", "dilara", laptop, "BLaptopKey"),
		} {
			if err := repo.Subscribe(t.Context(), subscription); err != nil {
				t.Fatalf("Subscribe() error = %v", err)
			}
		}

		found, err := repo.ListForUser(t.Context(), "dilara")
		if err != nil {
			t.Fatalf("ListForUser() error = %v", err)
		}
		if len(found) != 2 {
			t.Errorf("got %d subscriptions, want 2", len(found))
		}
	})

	// A browser that re-subscribes keeps its endpoint and gets new keys. Keeping
	// the old ones would mean encrypting to something that can no longer read it.
	t.Run("the same browser subscribing again replaces its keys", func(t *testing.T) {
		repo := newRepo(t)

		if err := repo.Subscribe(t.Context(), testSubscription(t, "s-1", "dilara", phone, "BOldKey")); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
		if err := repo.Subscribe(t.Context(), testSubscription(t, "s-2", "dilara", phone, "BNewKey")); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}

		found, err := repo.ListForUser(t.Context(), "dilara")
		if err != nil {
			t.Fatalf("ListForUser() error = %v", err)
		}
		if len(found) != 1 {
			t.Fatalf("got %d subscriptions, want 1 for one browser", len(found))
		}
		if found[0].P256dh != "BNewKey" {
			t.Errorf("key = %q, want the new one", found[0].P256dh)
		}
	})

	t.Run("subscriptions are kept apart by person", func(t *testing.T) {
		repo := newRepo(t)

		if err := repo.Subscribe(t.Context(), testSubscription(t, "s-1", "dilara", phone, "BKey")); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}

		found, err := repo.ListForUser(t.Context(), "mehmet")
		if err != nil {
			t.Fatalf("ListForUser() error = %v", err)
		}
		if len(found) != 0 {
			t.Errorf("somebody else's subscription came back: %v", found)
		}
	})

	// Also how a dead subscription is cleared: a push service saying an endpoint
	// is gone is telling the truth.
	t.Run("an endpoint can be forgotten", func(t *testing.T) {
		repo := newRepo(t)

		if err := repo.Subscribe(t.Context(), testSubscription(t, "s-1", "dilara", phone, "BKey")); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}

		if err := repo.Unsubscribe(t.Context(), phone); err != nil {
			t.Fatalf("Unsubscribe() error = %v", err)
		}

		found, _ := repo.ListForUser(t.Context(), "dilara")
		if len(found) != 0 {
			t.Errorf("got %d subscriptions after forgetting the only one", len(found))
		}

		// Forgetting one that is not there is not an error: the caller wanted it
		// gone either way.
		if err := repo.Unsubscribe(t.Context(), "https://push.example.com/nobody"); err != nil {
			t.Errorf("Unsubscribe() for an unknown endpoint = %v, want no error", err)
		}
	})
}
