package repository

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

type deliveryRepositoryFactory func(t *testing.T) (DeliveryRepository, NotificationRepository)

func newMemoryDeliveries(_ *testing.T) (DeliveryRepository, NotificationRepository) {
	return NewMemoryDeliveryRepository(), NewMemoryNotificationRepository()
}

func newPostgresDeliveries(t *testing.T) (DeliveryRepository, NotificationRepository) {
	t.Helper()

	pool := newTestPool(t)
	if _, err := pool.Exec(t.Context(), "truncate table notification_deliveries, notifications"); err != nil {
		t.Fatalf("resetting the outbox failed: %v", err)
	}

	return NewPostgresDeliveryRepository(pool), NewPostgresNotificationRepository(pool)
}

// seedNotice stores a notice, because a delivery points at one. In Postgres the
// foreign key insists; in memory nothing would, and the two have to be set up
// the same way or the tests are not testing the same thing.
func seedNotice(t *testing.T, notifications NotificationRepository, id, userID string, at time.Time) {
	t.Helper()

	notice, err := domain.NewNotification(
		id, userID, domain.NotifyEventCancelled, "event-1", "Radiohead", "A1", at)
	if err != nil {
		t.Fatalf("NewNotification() error = %v", err)
	}

	if err := notifications.Notify(t.Context(), notice); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestDeliveryRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo deliveryRepositoryFactory
	}{
		{"memory", newMemoryDeliveries},
		{"postgres", newPostgresDeliveries},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runDeliveryRepositoryContract(t, implementation.newRepo)
		})
	}
}

func runDeliveryRepositoryContract(t *testing.T, newRepo deliveryRepositoryFactory) {
	t.Helper()

	now := testTime()

	queue := func(t *testing.T, repo DeliveryRepository, notifications NotificationRepository,
		id, noticeID, userID string, channel domain.DeliveryChannel, due time.Time) *domain.Delivery {
		t.Helper()

		seedNotice(t, notifications, noticeID, userID, now)

		delivery, err := domain.NewDelivery(id, noticeID, userID, channel, now)
		if err != nil {
			t.Fatalf("NewDelivery() error = %v", err)
		}

		delivery.DueAt = due

		if err := repo.Enqueue(t.Context(), delivery); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}

		return delivery
	}

	t.Run("a due delivery is claimed and one that is not is left", func(t *testing.T) {
		repo, notifications := newRepo(t)

		queue(t, repo, notifications, "d-now", "n-now", "dilara", domain.ChannelEmail, now)
		queue(t, repo, notifications, "d-later", "n-later", "dilara", domain.ChannelEmail, now.Add(time.Hour))

		claimed, err := repo.TakeDue(t.Context(), now, 10)
		if err != nil {
			t.Fatalf("TakeDue() error = %v", err)
		}

		if len(claimed) != 1 || claimed[0].ID != "d-now" {
			t.Fatalf("claimed %v, want only d-now", claimed)
		}
		if claimed[0].Channel != domain.ChannelEmail || claimed[0].UserID != "dilara" {
			t.Errorf("claimed the wrong thing: %+v", claimed[0])
		}
	})

	// The lease. Claiming pushes the due time out, so a second pass arriving
	// straight away does not send the same message again.
	t.Run("a claimed delivery is not claimed again at once", func(t *testing.T) {
		repo, notifications := newRepo(t)

		queue(t, repo, notifications, "d-1", "n-1", "dilara", domain.ChannelEmail, now)

		if claimed, _ := repo.TakeDue(t.Context(), now, 10); len(claimed) != 1 {
			t.Fatalf("first pass claimed %d, want 1", len(claimed))
		}

		again, err := repo.TakeDue(t.Context(), now, 10)
		if err != nil {
			t.Fatalf("TakeDue() error = %v", err)
		}
		if len(again) != 0 {
			t.Errorf("second pass claimed %d, want none while the lease holds", len(again))
		}

		// Once the lease has run out it comes back, which is what saves a
		// delivery whose worker died before it could record anything.
		back, err := repo.TakeDue(t.Context(), now.Add(DeliveryLease+time.Second), 10)
		if err != nil {
			t.Fatalf("TakeDue() error = %v", err)
		}
		if len(back) != 1 {
			t.Errorf("after the lease, claimed %d, want 1", len(back))
		}
	})

	// The reason this is a claim and not a read: two workers must not both send
	// the same message.
	t.Run("concurrent workers never claim the same delivery", func(t *testing.T) {
		const count = 20

		repo, notifications := newRepo(t)

		for i := range count {
			queue(t, repo, notifications,
				fmt.Sprintf("d-%02d", i), fmt.Sprintf("n-%02d", i), "dilara", domain.ChannelEmail, now)
		}

		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			claims []string
		)

		for range count {
			wg.Add(1)

			go func() {
				defer wg.Done()

				claimed, err := repo.TakeDue(t.Context(), now, 1)
				if err != nil {
					return
				}

				mu.Lock()
				defer mu.Unlock()

				for _, delivery := range claimed {
					claims = append(claims, delivery.ID)
				}
			}()
		}

		wg.Wait()

		seen := make(map[string]bool, len(claims))
		for _, id := range claims {
			if seen[id] {
				t.Errorf("%s was claimed twice, so it would be sent twice", id)
			}

			seen[id] = true
		}
	})

	t.Run("a sent delivery is finished with", func(t *testing.T) {
		repo, notifications := newRepo(t)

		delivery := queue(t, repo, notifications, "d-1", "n-1", "dilara", domain.ChannelEmail, now)

		delivery.Sent(now)
		if err := repo.Save(t.Context(), delivery); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		claimed, err := repo.TakeDue(t.Context(), now.Add(time.Hour), 10)
		if err != nil {
			t.Fatalf("TakeDue() error = %v", err)
		}
		if len(claimed) != 0 {
			t.Errorf("claimed %d, want none; it has been sent", len(claimed))
		}

		pending, err := repo.Pending(t.Context())
		if err != nil {
			t.Fatalf("Pending() error = %v", err)
		}
		if pending != 0 {
			t.Errorf("pending = %d, want 0", pending)
		}
	})

	t.Run("a failed delivery comes back later and then gives up", func(t *testing.T) {
		repo, notifications := newRepo(t)

		delivery := queue(t, repo, notifications, "d-1", "n-1", "dilara", domain.ChannelEmail, now)

		delivery.Failed("connection refused", now)
		if err := repo.Save(t.Context(), delivery); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		// Not yet: the wait has not passed.
		if claimed, _ := repo.TakeDue(t.Context(), now, 10); len(claimed) != 0 {
			t.Errorf("claimed %d straight after a failure, want none", len(claimed))
		}

		claimed, err := repo.TakeDue(t.Context(), now.Add(2*domain.BaseRetryDelay), 10)
		if err != nil {
			t.Fatalf("TakeDue() error = %v", err)
		}
		if len(claimed) != 1 {
			t.Fatalf("claimed %d once the wait had passed, want 1", len(claimed))
		}
		if claimed[0].Attempts != 1 || claimed[0].LastError != "connection refused" {
			t.Errorf("the attempt was not recorded: %+v", claimed[0])
		}

		// Out of attempts: it stops coming back.
		delivery = claimed[0]
		for delivery.GaveUpAt.IsZero() {
			delivery.Failed("still refused", now)
		}

		if err := repo.Save(t.Context(), delivery); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		if claimed, _ := repo.TakeDue(t.Context(), now.Add(24*time.Hour), 10); len(claimed) != 0 {
			t.Errorf("claimed %d after giving up, want none", len(claimed))
		}
	})

	// Queueing the same notice and channel twice must not send it twice.
	t.Run("the same notice is queued once per channel", func(t *testing.T) {
		repo, notifications := newRepo(t)

		queue(t, repo, notifications, "d-1", "n-1", "dilara", domain.ChannelEmail, now)

		again, err := domain.NewDelivery("d-2", "n-1", "dilara", domain.ChannelEmail, now)
		if err != nil {
			t.Fatalf("NewDelivery() error = %v", err)
		}

		if err := repo.Enqueue(t.Context(), again); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}

		claimed, _ := repo.TakeDue(t.Context(), now, 10)
		if len(claimed) != 1 {
			t.Errorf("claimed %d, want 1; the second queueing is the same message", len(claimed))
		}
	})

	t.Run("saving something that is not queued is reported", func(t *testing.T) {
		repo, _ := newRepo(t)

		delivery, err := domain.NewDelivery("d-ghost", "n-ghost", "dilara", domain.ChannelEmail, now)
		if err != nil {
			t.Fatalf("NewDelivery() error = %v", err)
		}

		if err := repo.Save(t.Context(), delivery); !errors.Is(err, ErrDeliveryNotFound) {
			t.Errorf("Save() error = %v, want %v", err, ErrDeliveryNotFound)
		}
	})
}
