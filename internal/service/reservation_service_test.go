package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

const (
	testEventID = "event-1"
	testSeatID  = "A1"
	testUser    = "user-1"
	otherUser   = "user-2"
)

func testTime() time.Time {
	return time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC)
}

func newTestService(seats ...*domain.Seat) (*ReservationService, *fakeClock, *repository.MemorySeatRepository) {
	if len(seats) == 0 {
		seats = []*domain.Seat{domain.NewSeat(testEventID, testSeatID, "A", 1)}
	}

	clock := newFakeClock(testTime())
	repo := repository.NewMemorySeatRepository(seats...)

	svc := NewReservationService(Config{
		Seats:   repo,
		Events:  repository.NewMemoryEventRepository(testEvent()),
		Clock:   clock,
		NewID:   NewRandomID,
		HoldTTL: DefaultHoldTTL,
	})

	return svc, clock, repo
}

func testEvent() *domain.Event {
	return &domain.Event{
		ID:       testEventID,
		Name:     "Test Concert",
		Venue:    "Test Hall",
		StartsAt: testTime().Add(24 * time.Hour),
	}
}

func TestReservationService_HoldSeat(t *testing.T) {
	ctx := context.Background()

	t.Run("returns a hold describing the claim", func(t *testing.T) {
		svc, _, _ := newTestService()

		hold, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser)
		if err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}
		if hold.ID == "" {
			t.Error("hold id is empty, want a generated id")
		}
		if hold.UserID != testUser || hold.SeatID != testSeatID || hold.EventID != testEventID {
			t.Errorf("hold = %+v, want %q holding %q of %q", hold, testUser, testSeatID, testEventID)
		}
		if want := testTime().Add(DefaultHoldTTL); !hold.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", hold.ExpiresAt, want)
		}
		if !hold.CreatedAt.Equal(testTime()) {
			t.Errorf("CreatedAt = %v, want %v", hold.CreatedAt, testTime())
		}
	})

	t.Run("a held seat cannot be taken by someone else", func(t *testing.T) {
		svc, _, _ := newTestService()

		if _, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser); err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}

		_, err := svc.HoldSeat(ctx, testEventID, testSeatID, otherUser)
		if !errors.Is(err, domain.ErrSeatNotAvailable) {
			t.Errorf("HoldSeat() error = %v, want %v", err, domain.ErrSeatNotAvailable)
		}
	})

	t.Run("unknown seat returns ErrSeatNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()

		_, err := svc.HoldSeat(ctx, testEventID, "Z9", testUser)
		if !errors.Is(err, repository.ErrSeatNotFound) {
			t.Errorf("HoldSeat() error = %v, want %v", err, repository.ErrSeatNotFound)
		}
	})

	t.Run("the seat frees itself when the hold runs out", func(t *testing.T) {
		svc, clock, _ := newTestService()

		if _, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser); err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}

		clock.Advance(DefaultHoldTTL - time.Nanosecond)

		_, err := svc.HoldSeat(ctx, testEventID, testSeatID, otherUser)
		if !errors.Is(err, domain.ErrSeatNotAvailable) {
			t.Fatalf("one nanosecond before expiry: error = %v, want %v", err, domain.ErrSeatNotAvailable)
		}

		clock.Advance(time.Nanosecond)

		if _, err := svc.HoldSeat(ctx, testEventID, testSeatID, otherUser); err != nil {
			t.Errorf("at expiry: error = %v, want nil", err)
		}
	})
}

func TestReservationService_ConfirmReservation(t *testing.T) {
	ctx := context.Background()

	// holdSeat seeds a hold and returns its id.
	holdSeat := func(t *testing.T, svc *ReservationService, userID string) string {
		t.Helper()

		hold, err := svc.HoldSeat(ctx, testEventID, testSeatID, userID)
		if err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}

		return hold.ID
	}

	t.Run("the holder confirms and the seat is reserved", func(t *testing.T) {
		svc, _, _ := newTestService()
		holdID := holdSeat(t, svc, testUser)

		seat, err := svc.ConfirmReservation(ctx, holdID, testUser)
		if err != nil {
			t.Fatalf("ConfirmReservation() error = %v", err)
		}
		if seat.Status != domain.StatusReserved || seat.ReservedBy != testUser {
			t.Errorf("seat = %+v, want reserved by %q", seat, testUser)
		}
	})

	t.Run("another user cannot confirm the hold", func(t *testing.T) {
		svc, _, _ := newTestService()
		holdID := holdSeat(t, svc, testUser)

		_, err := svc.ConfirmReservation(ctx, holdID, otherUser)
		if !errors.Is(err, domain.ErrNotHoldOwner) {
			t.Errorf("ConfirmReservation() error = %v, want %v", err, domain.ErrNotHoldOwner)
		}
	})

	t.Run("an expired hold cannot be confirmed", func(t *testing.T) {
		svc, clock, _ := newTestService()
		holdID := holdSeat(t, svc, testUser)

		clock.Advance(DefaultHoldTTL)

		_, err := svc.ConfirmReservation(ctx, holdID, testUser)
		if !errors.Is(err, domain.ErrHoldExpired) {
			t.Errorf("ConfirmReservation() error = %v, want %v", err, domain.ErrHoldExpired)
		}
	})

	t.Run("unknown hold returns ErrHoldNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()

		_, err := svc.ConfirmReservation(ctx, "hold-does-not-exist", testUser)
		if !errors.Is(err, repository.ErrHoldNotFound) {
			t.Errorf("ConfirmReservation() error = %v, want %v", err, repository.ErrHoldNotFound)
		}
	})

	t.Run("a repeated confirm no longer finds the hold", func(t *testing.T) {
		svc, _, _ := newTestService()
		holdID := holdSeat(t, svc, testUser)

		if _, err := svc.ConfirmReservation(ctx, holdID, testUser); err != nil {
			t.Fatalf("ConfirmReservation() error = %v", err)
		}

		_, err := svc.ConfirmReservation(ctx, holdID, testUser)
		if !errors.Is(err, repository.ErrHoldNotFound) {
			t.Errorf("second ConfirmReservation() error = %v, want %v", err, repository.ErrHoldNotFound)
		}
	})
}

func TestReservationService_ReleaseSeat(t *testing.T) {
	ctx := context.Background()

	t.Run("the holder releases and the seat is free again", func(t *testing.T) {
		svc, _, _ := newTestService()

		hold, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser)
		if err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}

		if err := svc.ReleaseSeat(ctx, hold.ID, testUser); err != nil {
			t.Fatalf("ReleaseSeat() error = %v", err)
		}

		if _, err := svc.HoldSeat(ctx, testEventID, testSeatID, otherUser); err != nil {
			t.Errorf("HoldSeat() after release error = %v, want nil", err)
		}
	})

	t.Run("another user cannot release the hold", func(t *testing.T) {
		svc, _, _ := newTestService()

		hold, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser)
		if err != nil {
			t.Fatalf("HoldSeat() error = %v", err)
		}

		if err := svc.ReleaseSeat(ctx, hold.ID, otherUser); !errors.Is(err, domain.ErrNotHoldOwner) {
			t.Errorf("ReleaseSeat() error = %v, want %v", err, domain.ErrNotHoldOwner)
		}
	})

	t.Run("unknown hold returns ErrHoldNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()

		err := svc.ReleaseSeat(ctx, "hold-does-not-exist", testUser)
		if !errors.Is(err, repository.ErrHoldNotFound) {
			t.Errorf("ReleaseSeat() error = %v, want %v", err, repository.ErrHoldNotFound)
		}
	})
}

func TestReservationService_Reads(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(
		domain.NewSeat(testEventID, "A2", "A", 2),
		domain.NewSeat(testEventID, "A1", "A", 1),
	)

	t.Run("Seat returns one seat", func(t *testing.T) {
		got, err := svc.Seat(ctx, testEventID, "A1")
		if err != nil {
			t.Fatalf("Seat() error = %v", err)
		}
		if got.ID != "A1" {
			t.Errorf("seat = %q, want A1", got.ID)
		}
	})

	t.Run("SeatMap on an unknown event returns ErrEventNotFound", func(t *testing.T) {
		if _, err := svc.SeatMap(ctx, "no-such-event"); !errors.Is(err, repository.ErrEventNotFound) {
			t.Errorf("SeatMap() error = %v, want %v", err, repository.ErrEventNotFound)
		}
	})

	t.Run("SeatMap returns every seat in order", func(t *testing.T) {
		seats, err := svc.SeatMap(ctx, testEventID)
		if err != nil {
			t.Fatalf("SeatMap() error = %v", err)
		}
		if len(seats) != 2 || seats[0].ID != "A1" || seats[1].ID != "A2" {
			t.Errorf("seats = %+v, want A1 then A2", seats)
		}
	})
}

func TestReservationService_HoldSeat_OnlyOneConcurrentAttemptWins(t *testing.T) {
	const attempts = 100

	svc, _, _ := newTestService()
	ctx := context.Background()

	var (
		wg        sync.WaitGroup
		succeeded atomic.Int32
	)

	start := make(chan struct{})

	for i := range attempts {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			userID := fmt.Sprintf("user-%d", i)
			if _, err := svc.HoldSeat(ctx, testEventID, testSeatID, userID); err == nil {
				succeeded.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := succeeded.Load(); got != 1 {
		t.Fatalf("successful holds = %d, want exactly 1", got)
	}
}

func TestReservationService_ConfirmReservation_ConcurrentAttempts(t *testing.T) {
	const attempts = 50

	svc, _, _ := newTestService()
	ctx := context.Background()

	hold, err := svc.HoldSeat(ctx, testEventID, testSeatID, testUser)
	if err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	var (
		wg        sync.WaitGroup
		succeeded atomic.Int32
	)

	start := make(chan struct{})

	for range attempts {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			if _, err := svc.ConfirmReservation(ctx, hold.ID, testUser); err == nil {
				succeeded.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := succeeded.Load(); got != 1 {
		t.Fatalf("successful confirms = %d, want exactly 1", got)
	}
}
