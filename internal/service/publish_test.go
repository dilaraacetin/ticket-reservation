package service

import (
	"context"
	"sync"
	"testing"

	"ticket-reservation/internal/event"
	"ticket-reservation/internal/repository"
)

// recordingPublisher keeps what it was told, so a test can assert on it.
type recordingPublisher struct {
	mu     sync.Mutex
	events []event.Event
}

func (p *recordingPublisher) Publish(e event.Event) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = append(p.events, e)

	return 1
}

func (p *recordingPublisher) seen() []event.Event {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]event.Event(nil), p.events...)
}

func newPublishingService(t *testing.T) (*ReservationService, *recordingPublisher) {
	t.Helper()

	publisher := &recordingPublisher{}
	clock := newFakeClock(testTime())

	svc := NewReservationService(Config{
		Seats:     repository.NewMemorySeatRepository(newTestSeat()),
		Events:    repository.NewMemoryEventRepository(testEvent()),
		Clock:     clock,
		NewID:     NewRandomID,
		HoldTTL:   DefaultHoldTTL,
		Publisher: publisher,
	})

	return svc, publisher
}

// Every change to a seat has to be announced, or a watcher's map goes stale and
// the only fix is to poll again, which is what the stream exists to replace.
func TestReservationService_AnnouncesEverySeatChange(t *testing.T) {
	ctx := context.Background()

	t.Run("holding", func(t *testing.T) {
		svc, publisher := newPublishingService(t)

		if _, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser); err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}

		seen := publisher.seen()
		if len(seen) != 1 {
			t.Fatalf("published %d notices, want 1", len(seen))
		}
		if seen[0].Kind != event.SeatChanged || seen[0].SeatID != testSeatID {
			t.Errorf("notice = %+v, want a seat change for %s", seen[0], testSeatID)
		}

		// A seat map notice goes to everyone watching the event, so it must not
		// be addressed at anybody.
		if !seen[0].IsForEveryone() {
			t.Errorf("the notice is addressed at %q, want it public", seen[0].UserID)
		}
	})

	t.Run("confirming", func(t *testing.T) {
		svc, publisher := newPublishingService(t)

		hold, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser)
		if err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}
		if _, err := svc.ConfirmReservation(ctx, hold.ID, testUser); err != nil {
			t.Fatalf("ConfirmReservation() error = %v", err)
		}

		if got := len(publisher.seen()); got != 2 {
			t.Errorf("published %d notices, want 2", got)
		}
	})

	t.Run("releasing", func(t *testing.T) {
		svc, publisher := newPublishingService(t)

		hold, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser)
		if err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}
		if err := svc.ReleaseSeat(ctx, hold.ID, testUser); err != nil {
			t.Fatalf("ReleaseSeat() error = %v", err)
		}

		seen := publisher.seen()
		if len(seen) != 2 {
			t.Fatalf("published %d notices, want 2", len(seen))
		}
		if seen[1].SeatID != testSeatID {
			t.Errorf("the release notice names %q, want %q", seen[1].SeatID, testSeatID)
		}
	})
}

// A refused request changed nothing, so announcing one would send every watcher
// to fetch a map that is the same as the one they have.
func TestReservationService_SaysNothingWhenNothingChanged(t *testing.T) {
	ctx := context.Background()
	svc, publisher := newPublishingService(t)

	if _, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser); err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	before := len(publisher.seen())

	// Refused: the seat is already held.
	if _, err := svc.HoldSeat(ctx, testEventID, testSeatID, otherUser); err == nil {
		t.Fatal("HoldSeat() succeeded on a held seat")
	}

	// Refused: not the holder.
	if err := svc.ReleaseSeat(ctx, "no-such-hold", testUser); err == nil {
		t.Fatal("ReleaseSeat() succeeded for an unknown hold")
	}

	if got := len(publisher.seen()); got != before {
		t.Errorf("published %d notices for refused requests, want none", got-before)
	}
}

// A service built without a publisher must work, not panic.
func TestReservationService_WorksWithoutAPublisher(t *testing.T) {
	svc := NewReservationService(Config{
		Seats:   repository.NewMemorySeatRepository(newTestSeat()),
		Events:  repository.NewMemoryEventRepository(testEvent()),
		Clock:   newFakeClock(testTime()),
		NewID:   NewRandomID,
		HoldTTL: DefaultHoldTTL,
	})

	if _, err := svc.HoldSeat(context.Background(), testEventID, testSeatID, testUser); err != nil {
		t.Errorf("HoldSeat() error = %v", err)
	}
}
