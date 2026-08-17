package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

func event(id, name string, startsAt time.Time) *domain.Event {
	return &domain.Event{ID: id, Name: name, Venue: "Hall", StartsAt: startsAt}
}

func TestMemoryEventRepository_GetEvent(t *testing.T) {
	ctx := context.Background()
	now := testTime()
	repo := NewMemoryEventRepository(event("event-1", "Concert", now.Add(time.Hour)))

	t.Run("returns the stored event", func(t *testing.T) {
		got, err := repo.GetEvent(ctx, "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if got.Name != "Concert" {
			t.Errorf("name = %q, want Concert", got.Name)
		}
	})

	t.Run("unknown event returns ErrEventNotFound", func(t *testing.T) {
		if _, err := repo.GetEvent(ctx, "nope"); !errors.Is(err, ErrEventNotFound) {
			t.Errorf("GetEvent() error = %v, want %v", err, ErrEventNotFound)
		}
	})

	t.Run("mutating the result does not touch the store", func(t *testing.T) {
		got, err := repo.GetEvent(ctx, "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}

		got.Name = "Changed"

		again, err := repo.GetEvent(ctx, "event-1")
		if err != nil {
			t.Fatalf("GetEvent() error = %v", err)
		}
		if again.Name != "Concert" {
			t.Errorf("stored name = %q, want Concert", again.Name)
		}
	})
}

func TestMemoryEventRepository_ListEvents(t *testing.T) {
	ctx := context.Background()
	now := testTime()

	repo := NewMemoryEventRepository(
		event("late", "Late", now.Add(3*time.Hour)),
		event("early", "Early", now.Add(time.Hour)),
		event("middle", "Middle", now.Add(2*time.Hour)),
		event("also-middle", "Also Middle", now.Add(2*time.Hour)),
	)

	events, err := repo.ListEvents(ctx)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}

	want := []string{"early", "also-middle", "middle", "late"}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d", len(events), len(want))
	}

	for i, e := range events {
		if e.ID != want[i] {
			t.Errorf("event %d = %q, want %q", i, e.ID, want[i])
		}
	}
}
