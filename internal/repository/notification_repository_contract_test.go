package repository

import (
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

type notificationRepositoryFactory func(t *testing.T) NotificationRepository

func newMemoryNotifications(_ *testing.T) NotificationRepository {
	return NewMemoryNotificationRepository()
}

func newPostgresNotifications(t *testing.T) NotificationRepository {
	t.Helper()

	pool := newTestPool(t)
	// Deliveries reference notices, so the two go together.
	if _, err := pool.Exec(t.Context(), "truncate table notification_deliveries, notifications"); err != nil {
		t.Fatalf("resetting notifications failed: %v", err)
	}

	return NewPostgresNotificationRepository(pool)
}

func testNotice(t *testing.T, id, userID, seatID string, at time.Time) *domain.Notification {
	t.Helper()

	notice, err := domain.NewNotification(
		id, userID, domain.NotifyEventCancelled, "event-1", "Radiohead", seatID, at)
	if err != nil {
		t.Fatalf("NewNotification() error = %v", err)
	}

	return notice
}

func TestNotificationRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo notificationRepositoryFactory
	}{
		{"memory", newMemoryNotifications},
		{"postgres", newPostgresNotifications},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runNotificationRepositoryContract(t, implementation.newRepo)
		})
	}
}

func runNotificationRepositoryContract(t *testing.T, newRepo notificationRepositoryFactory) {
	t.Helper()

	now := testTime()

	t.Run("somebody with nothing to read gets an empty slice", func(t *testing.T) {
		repo := newRepo(t)

		notices, err := repo.ListForUser(t.Context(), "nobody")
		if err != nil {
			t.Fatalf("ListForUser() error = %v", err)
		}
		if notices == nil {
			t.Error("got nil, want an empty slice so the API answers [] rather than null")
		}
		if len(notices) != 0 {
			t.Errorf("got %d notices for somebody with none", len(notices))
		}
	})

	t.Run("notices come back newest first", func(t *testing.T) {
		repo := newRepo(t)

		err := repo.Notify(t.Context(),
			testNotice(t, "n-old", "dilara", "A1", now),
			testNotice(t, "n-new", "dilara", "A2", now.Add(time.Hour)),
		)
		if err != nil {
			t.Fatalf("Notify() error = %v", err)
		}

		notices, err := repo.ListForUser(t.Context(), "dilara")
		if err != nil {
			t.Fatalf("ListForUser() error = %v", err)
		}
		if len(notices) != 2 {
			t.Fatalf("got %d notices, want 2", len(notices))
		}
		if notices[0].ID != "n-new" {
			t.Errorf("first notice is %q, want the newest", notices[0].ID)
		}

		// Everything the reader needs, including the event's name: the event may
		// be gone by now, and "your ticket to something" is not a message.
		if notices[0].EventName != "Radiohead" || notices[0].SeatID != "A2" {
			t.Errorf("notice is missing its detail: %+v", notices[0])
		}
		if notices[0].Kind != domain.NotifyEventCancelled {
			t.Errorf("kind = %q, want %q", notices[0].Kind, domain.NotifyEventCancelled)
		}
	})

	// One person's notices are not another's.
	t.Run("notices are kept apart by person", func(t *testing.T) {
		repo := newRepo(t)

		err := repo.Notify(t.Context(),
			testNotice(t, "n-mine", "dilara", "A1", now),
			testNotice(t, "n-theirs", "mehmet", "A2", now),
		)
		if err != nil {
			t.Fatalf("Notify() error = %v", err)
		}

		mine, err := repo.ListForUser(t.Context(), "dilara")
		if err != nil {
			t.Fatalf("ListForUser() error = %v", err)
		}
		if len(mine) != 1 || mine[0].ID != "n-mine" {
			t.Errorf("got %v, want only n-mine", mine)
		}
	})

	t.Run("a new notice is unread until it is marked", func(t *testing.T) {
		repo := newRepo(t)

		if err := repo.Notify(t.Context(), testNotice(t, "n-1", "dilara", "A1", now)); err != nil {
			t.Fatalf("Notify() error = %v", err)
		}

		notices, _ := repo.ListForUser(t.Context(), "dilara")
		if notices[0].IsRead() {
			t.Fatal("a notice nobody has opened reads as read")
		}

		marked, err := repo.MarkAllRead(t.Context(), "dilara", now.Add(time.Minute))
		if err != nil {
			t.Fatalf("MarkAllRead() error = %v", err)
		}
		if marked != 1 {
			t.Errorf("marked %d, want 1", marked)
		}

		notices, _ = repo.ListForUser(t.Context(), "dilara")
		if !notices[0].IsRead() {
			t.Error("the notice is still unread after being marked")
		}

		// Marking twice marks nothing: there is nothing left unread.
		marked, err = repo.MarkAllRead(t.Context(), "dilara", now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("MarkAllRead() error = %v", err)
		}
		if marked != 0 {
			t.Errorf("marked %d on the second pass, want 0", marked)
		}
	})

	// Opening your own list must not touch anybody else's.
	t.Run("marking read only touches one person", func(t *testing.T) {
		repo := newRepo(t)

		err := repo.Notify(t.Context(),
			testNotice(t, "n-mine", "dilara", "A1", now),
			testNotice(t, "n-theirs", "mehmet", "A2", now),
		)
		if err != nil {
			t.Fatalf("Notify() error = %v", err)
		}

		if _, err := repo.MarkAllRead(t.Context(), "dilara", now.Add(time.Minute)); err != nil {
			t.Fatalf("MarkAllRead() error = %v", err)
		}

		theirs, _ := repo.ListForUser(t.Context(), "mehmet")
		if theirs[0].IsRead() {
			t.Error("marking one person's notices read marked another's")
		}
	})

	t.Run("notifying nobody is not an error", func(t *testing.T) {
		repo := newRepo(t)

		if err := repo.Notify(t.Context()); err != nil {
			t.Errorf("Notify() with no notices = %v, want no error", err)
		}
	})
}
