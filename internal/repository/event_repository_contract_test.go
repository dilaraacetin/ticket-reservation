package repository

import (
	"errors"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

type eventRepositoryFactory func(t *testing.T, events ...*domain.Event) EventRepository

func newMemoryEvents(_ *testing.T, events ...*domain.Event) EventRepository {
	return NewMemoryEventRepository(events...)
}

func newPostgresEvents(t *testing.T, events ...*domain.Event) EventRepository {
	t.Helper()

	pool := newTestPool(t)
	resetSchema(t, pool)

	repo := NewPostgresEventRepository(pool)
	if len(events) > 0 {
		if err := repo.InsertEvents(t.Context(), events...); err != nil {
			t.Fatalf("seeding events failed: %v", err)
		}
	}

	return repo
}

func testEvent(id, name string, startsAt time.Time) *domain.Event {
	return &domain.Event{ID: id, Name: name, Venue: "Hall", StartsAt: startsAt}
}

func TestEventRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo eventRepositoryFactory
	}{
		{"memory", newMemoryEvents},
		{"postgres", newPostgresEvents},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runEventRepositoryContract(t, implementation.newRepo)
		})
	}
}

func runEventRepositoryContract(t *testing.T, newRepo eventRepositoryFactory) {
	t.Helper()

	now := testTime()

	t.Run("GetEvent returns the stored event", func(t *testing.T) {
		repo := newRepo(t, testEvent("event-1", "Concert", now.Add(time.Hour)))

		got, err := repo.GetEvent(t.Context(), "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if got.Name != "Concert" || got.Venue != "Hall" {
			t.Errorf("event = %+v, want Concert at Hall", got)
		}
		if !got.StartsAt.Equal(now.Add(time.Hour)) {
			t.Errorf("StartsAt = %v, want %v", got.StartsAt, now.Add(time.Hour))
		}
	})

	t.Run("GetEvent on an unknown event returns ErrEventNotFound", func(t *testing.T) {
		repo := newRepo(t)

		if _, err := repo.GetEvent(t.Context(), "no-such-event"); !errors.Is(err, ErrEventNotFound) {
			t.Errorf("GetEvent() error = %v, want %v", err, ErrEventNotFound)
		}
	})

	t.Run("mutating a returned event does not touch the store", func(t *testing.T) {
		repo := newRepo(t, testEvent("event-1", "Concert", now.Add(time.Hour)))

		got, err := repo.GetEvent(t.Context(), "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}

		got.Name = "Changed"

		again, err := repo.GetEvent(t.Context(), "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if again.Name != "Concert" {
			t.Errorf("stored name = %q, want Concert", again.Name)
		}
	})

	t.Run("ListEvents returns them soonest first", func(t *testing.T) {
		repo := newRepo(t,
			testEvent("late", "Late", now.Add(3*time.Hour)),
			testEvent("early", "Early", now.Add(time.Hour)),
			testEvent("middle", "Middle", now.Add(2*time.Hour)),
			testEvent("also-middle", "Also Middle", now.Add(2*time.Hour)),
		)

		events, err := repo.ListEvents(t.Context())
		if err != nil {
			t.Fatalf("ListEvents() error = %v", err)
		}

		want := []string{"early", "also-middle", "middle", "late"}
		if len(events) != len(want) {
			t.Fatalf("got %d events, want %d", len(events), len(want))
		}

		for i, event := range events {
			if event.ID != want[i] {
				t.Errorf("event %d = %q, want %q", i, event.ID, want[i])
			}
		}
	})

	t.Run("an empty catalogue is an empty slice, not nil", func(t *testing.T) {
		repo := newRepo(t)

		events, err := repo.ListEvents(t.Context())
		if err != nil {
			t.Fatalf("ListEvents() error = %v", err)
		}
		if events == nil {
			t.Error("ListEvents() = nil, want an empty slice")
		}
		if len(events) != 0 {
			t.Errorf("got %d events, want 0", len(events))
		}
	})
}
