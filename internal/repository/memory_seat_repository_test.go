package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

const (
	testEventID = "event-1"
	testSeatID  = "A1"
	testHoldID  = "hold-1"
	testUser    = "user-1"
	holdTTL     = 5 * time.Minute
)

func testTime() time.Time {
	return time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC)
}

func seat(eventID, seatID, row string, number int) *domain.Seat {
	return domain.NewSeat(eventID, seatID, row, number)
}

func TestMemorySeatRepository_GetSeat(t *testing.T) {
	ctx := context.Background()
	repo := NewMemorySeatRepository(seat(testEventID, testSeatID, "A", 1))

	t.Run("returns the stored seat", func(t *testing.T) {
		got, err := repo.GetSeat(ctx, testEventID, testSeatID)
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.ID != testSeatID || got.EventID != testEventID {
			t.Errorf("seat = %+v, want %q of %q", got, testSeatID, testEventID)
		}
	})

	t.Run("unknown seat returns ErrSeatNotFound", func(t *testing.T) {
		if _, err := repo.GetSeat(ctx, testEventID, "Z9"); !errors.Is(err, ErrSeatNotFound) {
			t.Errorf("GetSeat() error = %v, want %v", err, ErrSeatNotFound)
		}
	})

	t.Run("mutating the result does not touch the store", func(t *testing.T) {
		got, err := repo.GetSeat(ctx, testEventID, testSeatID)
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}

		got.Status = domain.StatusReserved

		again, err := repo.GetSeat(ctx, testEventID, testSeatID)
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if again.Status != domain.StatusAvailable {
			t.Errorf("stored status = %v, want %v", again.Status, domain.StatusAvailable)
		}
	})
}

func TestMemorySeatRepository_UpdateSeat(t *testing.T) {
	ctx := context.Background()

	t.Run("applies the mutation", func(t *testing.T) {
		repo := NewMemorySeatRepository(seat(testEventID, testSeatID, "A", 1))

		err := repo.UpdateSeat(ctx, testEventID, testSeatID, func(s *domain.Seat) error {
			return s.Hold(testHoldID, testUser, holdTTL, testTime())
		})
		if err != nil {
			t.Fatalf("UpdateSeat() error = %v", err)
		}

		got, err := repo.GetSeat(ctx, testEventID, testSeatID)
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.Status != domain.StatusHeld || got.HeldBy != testUser {
			t.Errorf("seat = %+v, want held by %q", got, testUser)
		}
	})

	t.Run("a failing mutation leaves the store untouched", func(t *testing.T) {
		repo := NewMemorySeatRepository(seat(testEventID, testSeatID, "A", 1))
		wantErr := errors.New("mutation refused")

		err := repo.UpdateSeat(ctx, testEventID, testSeatID, func(s *domain.Seat) error {
			s.Status = domain.StatusReserved // a half applied change

			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("UpdateSeat() error = %v, want %v", err, wantErr)
		}

		got, err := repo.GetSeat(ctx, testEventID, testSeatID)
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.Status != domain.StatusAvailable {
			t.Errorf("stored status = %v, want %v", got.Status, domain.StatusAvailable)
		}
	})

	t.Run("unknown seat returns ErrSeatNotFound", func(t *testing.T) {
		repo := NewMemorySeatRepository()

		err := repo.UpdateSeat(ctx, testEventID, "Z9", func(*domain.Seat) error {
			t.Error("mutate must not be called for a missing seat")

			return nil
		})
		if !errors.Is(err, ErrSeatNotFound) {
			t.Errorf("UpdateSeat() error = %v, want %v", err, ErrSeatNotFound)
		}
	})
}

func TestMemorySeatRepository_UpdateSeatByHoldID(t *testing.T) {
	ctx := context.Background()

	newHeldRepo := func(t *testing.T) *MemorySeatRepository {
		t.Helper()

		repo := NewMemorySeatRepository(seat(testEventID, testSeatID, "A", 1))
		err := repo.UpdateSeat(ctx, testEventID, testSeatID, func(s *domain.Seat) error {
			return s.Hold(testHoldID, testUser, holdTTL, testTime())
		})
		if err != nil {
			t.Fatalf("seeding hold failed: %v", err)
		}

		return repo
	}

	t.Run("finds the seat carrying the hold", func(t *testing.T) {
		repo := newHeldRepo(t)

		err := repo.UpdateSeatByHoldID(ctx, testHoldID, func(s *domain.Seat) error {
			return s.Confirm(testUser, testTime().Add(time.Minute))
		})
		if err != nil {
			t.Fatalf("UpdateSeatByHoldID() error = %v", err)
		}

		got, err := repo.GetSeat(ctx, testEventID, testSeatID)
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.Status != domain.StatusReserved {
			t.Errorf("status = %v, want %v", got.Status, domain.StatusReserved)
		}
	})

	t.Run("a failing mutation leaves the store untouched", func(t *testing.T) {
		repo := newHeldRepo(t)
		wantErr := errors.New("mutation refused")

		err := repo.UpdateSeatByHoldID(ctx, testHoldID, func(s *domain.Seat) error {
			s.Status = domain.StatusReserved

			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("UpdateSeatByHoldID() error = %v, want %v", err, wantErr)
		}

		got, err := repo.GetSeat(ctx, testEventID, testSeatID)
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.Status != domain.StatusHeld {
			t.Errorf("stored status = %v, want %v", got.Status, domain.StatusHeld)
		}
	})

	t.Run("unknown hold returns ErrHoldNotFound", func(t *testing.T) {
		repo := newHeldRepo(t)

		err := repo.UpdateSeatByHoldID(ctx, "hold-does-not-exist", func(*domain.Seat) error {
			t.Error("mutate must not be called for a missing hold")

			return nil
		})
		if !errors.Is(err, ErrHoldNotFound) {
			t.Errorf("UpdateSeatByHoldID() error = %v, want %v", err, ErrHoldNotFound)
		}
	})
}

func TestMemorySeatRepository_ExpireHolds(t *testing.T) {
	ctx := context.Background()
	now := testTime()

	repo := NewMemorySeatRepository(
		seat(testEventID, "A1", "A", 1),
		seat(testEventID, "A2", "A", 2),
		seat(testEventID, "A3", "A", 3),
	)

	// A1 expires within the minute, A2 lasts an hour, A3 stays available.
	hold := func(seatID, holdID string, ttl time.Duration) {
		t.Helper()

		err := repo.UpdateSeat(ctx, testEventID, seatID, func(s *domain.Seat) error {
			return s.Hold(holdID, testUser, ttl, now)
		})
		if err != nil {
			t.Fatalf("seeding hold on %s failed: %v", seatID, err)
		}
	}
	hold("A1", "hold-a1", time.Minute)
	hold("A2", "hold-a2", time.Hour)

	expired, err := repo.ExpireHolds(ctx, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ExpireHolds() error = %v", err)
	}
	if expired != 1 {
		t.Errorf("expired = %d, want 1", expired)
	}

	seats, err := repo.ListSeats(ctx, testEventID)
	if err != nil {
		t.Fatalf("ListSeats() error = %v", err)
	}

	wantStatus := []domain.SeatStatus{domain.StatusAvailable, domain.StatusHeld, domain.StatusAvailable}
	for i, s := range seats {
		if s.Status != wantStatus[i] {
			t.Errorf("%s status = %v, want %v", s.ID, s.Status, wantStatus[i])
		}
	}
}

func TestMemorySeatRepository_ListSeats(t *testing.T) {
	ctx := context.Background()
	repo := NewMemorySeatRepository(
		seat(testEventID, "B2", "B", 2),
		seat(testEventID, "A10", "A", 10),
		seat(testEventID, "A2", "A", 2),
		seat("event-2", "A1", "A", 1),
	)

	seats, err := repo.ListSeats(ctx, testEventID)
	if err != nil {
		t.Fatalf("ListSeats() error = %v", err)
	}

	// Ordered by row then number, so A10 comes after A2 rather than sorting as
	// a string would put it.
	want := []string{"A2", "A10", "B2"}
	if len(seats) != len(want) {
		t.Fatalf("got %d seats, want %d", len(seats), len(want))
	}

	for i, s := range seats {
		if s.ID != want[i] {
			t.Errorf("seat %d = %q, want %q", i, s.ID, want[i])
		}
	}
}

func TestMemorySeatRepository_ConcurrentUpdateSeat(t *testing.T) {
	const attempts = 100

	ctx := context.Background()
	repo := NewMemorySeatRepository(seat(testEventID, testSeatID, "A", 1))
	now := testTime()

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

			err := repo.UpdateSeat(ctx, testEventID, testSeatID, func(s *domain.Seat) error {
				return s.Hold(fmt.Sprintf("hold-%d", i), fmt.Sprintf("user-%d", i), holdTTL, now)
			})
			if err == nil {
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
