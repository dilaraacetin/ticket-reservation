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
	return &domain.Event{
		ID:       id,
		Name:     name,
		Venue:    "Volkswagen Arena",
		StartsAt: startsAt,
		Details: domain.EventDetails{
			City:        "Istanbul",
			Category:    domain.CategoryConcert,
			ImageURL:    "https://images.seathold.test/" + id + ".jpg",
			Description: "Doors at 19:00, support act at 20:00, main set at 21:15.",
			Rules:       "No professional cameras. Under 16s must come with an adult.",
		},
	}
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

	t.Run("CreateEvent stores an event that can then be read", func(t *testing.T) {
		repo := newRepo(t)

		event := testEvent("event-created", "Radiohead", now.Add(72*time.Hour))
		if err := repo.CreateEvent(t.Context(), event); err != nil {
			t.Fatalf("CreateEvent() error = %v", err)
		}

		stored, err := repo.GetEvent(t.Context(), event.ID)
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if stored.Name != event.Name || stored.Venue != event.Venue {
			t.Errorf("stored %q at %q, want %q at %q", stored.Name, stored.Venue, event.Name, event.Venue)
		}
		if !stored.StartsAt.Equal(event.StartsAt) {
			t.Errorf("StartsAt = %s, want %s", stored.StartsAt, event.StartsAt)
		}
	})

	// The descriptive columns are written and read by their own list, apart from
	// the booking ones, so they get their own round trip.
	t.Run("the details survive a write and a read", func(t *testing.T) {
		repo := newRepo(t)

		event := testEvent("event-details", "Radiohead", now.Add(72*time.Hour))
		if err := repo.CreateEvent(t.Context(), event); err != nil {
			t.Fatalf("CreateEvent() error = %v", err)
		}

		stored, err := repo.GetEvent(t.Context(), event.ID)
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if stored.Details != event.Details {
			t.Errorf("Details = %+v, want %+v", stored.Details, event.Details)
		}

		// Clearing one is the case the "taken as written" rule exists for, and
		// the one a wrong argument order would quietly leave untouched.
		wanted := domain.EventDetails{
			City:        "Ankara",
			Category:    domain.CategoryFestival,
			Description: "Rescheduled from October.",
		}

		err = repo.UpdateEvent(t.Context(), event.ID, func(event *domain.Event) error {
			return event.Update("", "", time.Time{}, wanted)
		})
		if err != nil {
			t.Fatalf("UpdateEvent() error = %v", err)
		}

		stored, err = repo.GetEvent(t.Context(), event.ID)
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if stored.Details != wanted {
			t.Errorf("Details = %+v, want %+v", stored.Details, wanted)
		}
	})

	// Both stores have to answer this the same way. Postgres fills the default in
	// because the column cannot be null; a store that kept the empty string
	// instead would hand back an event with no category at all.
	t.Run("an event stored without a category reads back as the default", func(t *testing.T) {
		repo := newRepo(t)

		event := testEvent("event-uncategorised", "Radiohead", now.Add(72*time.Hour))
		event.Details.Category = ""

		if err := repo.CreateEvent(t.Context(), event); err != nil {
			t.Fatalf("CreateEvent() error = %v", err)
		}

		stored, err := repo.GetEvent(t.Context(), event.ID)
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if stored.Details.Category != domain.DefaultCategory {
			t.Errorf("Category = %q, want %q", stored.Details.Category, domain.DefaultCategory)
		}
	})

	// A clash has to be reported rather than ignored, or a request asking for an
	// id that is taken appears to have worked.
	t.Run("CreateEvent reports an id that is already taken", func(t *testing.T) {
		repo := newRepo(t)

		event := testEvent("event-twice", "Radiohead", now.Add(72*time.Hour))
		if err := repo.CreateEvent(t.Context(), event); err != nil {
			t.Fatalf("CreateEvent() error = %v", err)
		}

		second := testEvent("event-twice", "Someone Else", now.Add(96*time.Hour))
		if err := repo.CreateEvent(t.Context(), second); !errors.Is(err, ErrEventExists) {
			t.Fatalf("CreateEvent() error = %v, want %v", err, ErrEventExists)
		}

		// And the first one is still what is stored.
		stored, err := repo.GetEvent(t.Context(), event.ID)
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if stored.Name != "Radiohead" {
			t.Errorf("stored name = %q, want the original Radiohead", stored.Name)
		}
	})

	t.Run("GetEvent returns the stored event", func(t *testing.T) {
		repo := newRepo(t, testEvent("event-1", "Radiohead", now.Add(time.Hour)))

		got, err := repo.GetEvent(t.Context(), "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if got.Name != "Radiohead" || got.Venue != "Volkswagen Arena" {
			t.Errorf("event = %+v, want Radiohead at Volkswagen Arena", got)
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
		repo := newRepo(t, testEvent("event-1", "Radiohead", now.Add(time.Hour)))

		got, err := repo.GetEvent(t.Context(), "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}

		got.Name = "Coldplay"

		again, err := repo.GetEvent(t.Context(), "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if again.Name != "Radiohead" {
			t.Errorf("stored name = %q, want Radiohead", again.Name)
		}
	})

	t.Run("ListEvents returns them soonest first", func(t *testing.T) {
		repo := newRepo(t,
			// The ids say where each event belongs in the expected order, which is
			// what the assertion below reads. The names are only there to look like
			// the real thing.
			testEvent("late", "Massive Attack", now.Add(3*time.Hour)),
			testEvent("early", "Radiohead", now.Add(time.Hour)),
			testEvent("middle", "Sigur Ros", now.Add(2*time.Hour)),
			testEvent("also-middle", "Portishead", now.Add(2*time.Hour)),
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

// eventStores pairs an event store with the seat store it has to agree with, so
// the two implementations of "refuse to delete while a seat is spoken for" can
// be held to the same answer. They are written very differently: the memory
// store walks its seats, Postgres does it in one statement.
type eventStores struct {
	events EventRepository
	seats  SeatRepository
}

func newMemoryEventStores(_ *testing.T, eventID string, seats ...*domain.Seat) eventStores {
	seatStore := NewMemorySeatRepository(seats...)
	event := testEvent(eventID, "Radiohead", testTime().Add(72*time.Hour))

	return eventStores{
		events: NewMemoryEventRepository(event).WithSeats(seatStore),
		seats:  seatStore,
	}
}

func newPostgresEventStores(t *testing.T, eventID string, seats ...*domain.Seat) eventStores {
	t.Helper()

	pool := seededPool(t, eventID)

	seatStore := NewPostgresSeatRepository(pool)
	if _, err := seatStore.CreateSeats(t.Context(), seats...); err != nil {
		t.Fatalf("seeding seats failed: %v", err)
	}

	return eventStores{events: NewPostgresEventRepository(pool), seats: seatStore}
}

func TestEventRepositoryContract_Withdrawal(t *testing.T) {
	implementations := []struct {
		name      string
		newStores func(t *testing.T, eventID string, seats ...*domain.Seat) eventStores
	}{
		{"memory", newMemoryEventStores},
		{"postgres", newPostgresEventStores},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runEventWithdrawalContract(t, implementation.newStores)
		})
	}
}

func runEventWithdrawalContract(
	t *testing.T,
	newStores func(t *testing.T, eventID string, seats ...*domain.Seat) eventStores,
) {
	t.Helper()

	now := testTime()

	t.Run("an event with nothing held can be deleted", func(t *testing.T) {
		eventID := uniqueEventID(t)
		stores := newStores(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))

		if err := stores.events.DeleteEvent(t.Context(), eventID); err != nil {
			t.Fatalf("DeleteEvent() error = %v", err)
		}

		if _, err := stores.events.GetEvent(t.Context(), eventID); !errors.Is(err, ErrEventNotFound) {
			t.Errorf("GetEvent() after the delete = %v, want %v", err, ErrEventNotFound)
		}
	})

	t.Run("an event with a held seat is not deleted", func(t *testing.T) {
		eventID := uniqueEventID(t)
		stores := newStores(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))

		err := stores.seats.UpdateSeat(t.Context(), eventID, "A1", func(seat *domain.Seat) error {
			return seat.Hold("hold-1", "user-1", time.Hour, now)
		})
		if err != nil {
			t.Fatalf("holding A1 failed: %v", err)
		}

		if err := stores.events.DeleteEvent(t.Context(), eventID); !errors.Is(err, ErrEventInUse) {
			t.Fatalf("DeleteEvent() = %v, want %v", err, ErrEventInUse)
		}

		// And it is still there, rather than half gone.
		if _, err := stores.events.GetEvent(t.Context(), eventID); err != nil {
			t.Errorf("GetEvent() after the refusal = %v, want the event intact", err)
		}
	})

	t.Run("deleting an event that is not there is reported", func(t *testing.T) {
		stores := newStores(t, uniqueEventID(t))

		if err := stores.events.DeleteEvent(t.Context(), "no-such-event"); !errors.Is(err, ErrEventNotFound) {
			t.Errorf("DeleteEvent() = %v, want %v", err, ErrEventNotFound)
		}
	})

	t.Run("cancelling is stored and read back", func(t *testing.T) {
		eventID := uniqueEventID(t)
		stores := newStores(t, eventID)

		err := stores.events.UpdateEvent(t.Context(), eventID, func(event *domain.Event) error {
			return event.Cancel(now)
		})
		if err != nil {
			t.Fatalf("UpdateEvent() error = %v", err)
		}

		stored, err := stores.events.GetEvent(t.Context(), eventID)
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if !stored.IsCancelled() {
			t.Fatal("the event does not read back as cancelled")
		}
		if !stored.CancelledAt.Equal(now.UTC().Truncate(time.Second)) {
			t.Errorf("CancelledAt = %s, want %s", stored.CancelledAt, now.UTC().Truncate(time.Second))
		}
	})

	// A mutate that fails must leave the stored event exactly as it was.
	t.Run("a refused change writes nothing", func(t *testing.T) {
		eventID := uniqueEventID(t)
		stores := newStores(t, eventID)

		err := stores.events.UpdateEvent(t.Context(), eventID, func(event *domain.Event) error {
			event.Name = "Half Written"

			return domain.ErrInvalidEvent
		})
		if !errors.Is(err, domain.ErrInvalidEvent) {
			t.Fatalf("UpdateEvent() error = %v, want %v", err, domain.ErrInvalidEvent)
		}

		stored, err := stores.events.GetEvent(t.Context(), eventID)
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if stored.Name == "Half Written" {
			t.Error("a change the mutate refused was written anyway")
		}
	})
}
