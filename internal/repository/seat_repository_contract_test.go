package repository

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/domain"
)

const (
	testHoldID = "hold-1"
	testUser   = "user-1"
	holdTTL    = 5 * time.Minute
)

func testTime() time.Time {
	return time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC)
}

type seatRepositoryFactory func(t *testing.T, eventID string, seats ...*domain.Seat) SeatRepository

func newMemorySeats(_ *testing.T, _ string, seats ...*domain.Seat) SeatRepository {
	return NewMemorySeatRepository(seats...)
}

func newPostgresSeats(t *testing.T, eventID string, seats ...*domain.Seat) SeatRepository {
	t.Helper()

	pool := seededPool(t, eventID)
	repo := NewPostgresSeatRepository(pool)

	if err := repo.InsertSeats(t.Context(), seats...); err != nil {
		t.Fatalf("seeding seats failed: %v", err)
	}

	return repo
}

func newOptimisticSeats(t *testing.T, eventID string, seats ...*domain.Seat) SeatRepository {
	t.Helper()

	pool := seededPool(t, eventID)
	repo := NewOptimisticSeatRepository(pool, DefaultUpdateAttempts)

	if err := repo.InsertSeats(t.Context(), seats...); err != nil {
		t.Fatalf("seeding seats failed: %v", err)
	}

	return repo
}

func seededPool(t *testing.T, eventID string) *pgxpool.Pool {
	t.Helper()

	pool := newTestPool(t)
	resetSchema(t, pool)

	event := &domain.Event{
		ID:       eventID,
		Name:     "Radiohead",
		Venue:    "Volkswagen Arena",
		StartsAt: testTime().Add(time.Hour),
	}
	if err := NewPostgresEventRepository(pool).InsertEvents(t.Context(), event); err != nil {
		t.Fatalf("seeding the event failed: %v", err)
	}

	return pool
}

func TestSeatRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo seatRepositoryFactory
	}{
		{"memory", newMemorySeats},
		{"postgres-pessimistic", newPostgresSeats},
		{"postgres-optimistic", newOptimisticSeats},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runSeatRepositoryContract(t, implementation.newRepo)
		})
	}
}

func runSeatRepositoryContract(t *testing.T, newRepo seatRepositoryFactory) {
	t.Helper()

	t.Run("GetSeat returns the stored seat", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))

		got, err := repo.GetSeat(t.Context(), eventID, "A1")
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.ID != "A1" || got.EventID != eventID {
			t.Errorf("seat = %+v, want A1 of %s", got, eventID)
		}
		if got.Row != "A" || got.Number != 1 {
			t.Errorf("row/number = %q/%d, want A/1", got.Row, got.Number)
		}
		if got.Status != domain.StatusAvailable {
			t.Errorf("status = %v, want %v", got.Status, domain.StatusAvailable)
		}
	})

	t.Run("GetSeat on an unknown seat returns ErrSeatNotFound", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		if _, err := repo.GetSeat(t.Context(), eventID, "Z9"); !errors.Is(err, ErrSeatNotFound) {
			t.Errorf("GetSeat() error = %v, want %v", err, ErrSeatNotFound)
		}
	})

	t.Run("mutating a returned seat does not touch the store", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))

		got, err := repo.GetSeat(t.Context(), eventID, "A1")
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}

		got.Status = domain.StatusReserved
		got.ReservedBy = "nobody"

		again, err := repo.GetSeat(t.Context(), eventID, "A1")
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if again.Status != domain.StatusAvailable {
			t.Errorf("stored status = %v, want %v", again.Status, domain.StatusAvailable)
		}
	})

	t.Run("UpdateSeat applies the mutation", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))
		now := testTime()

		err := repo.UpdateSeat(t.Context(), eventID, "A1", func(seat *domain.Seat) error {
			return seat.Hold(testHoldID, testUser, holdTTL, now)
		})
		if err != nil {
			t.Fatalf("UpdateSeat() error = %v", err)
		}

		got, err := repo.GetSeat(t.Context(), eventID, "A1")
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.Status != domain.StatusHeld || got.HeldBy != testUser || got.HoldID != testHoldID {
			t.Errorf("seat = %+v, want held by %q under %q", got, testUser, testHoldID)
		}

		if !got.HoldCreatedAt.Equal(now) {
			t.Errorf("HoldCreatedAt = %v, want %v", got.HoldCreatedAt, now)
		}
		if want := now.Add(holdTTL); !got.HoldExpiresAt.Equal(want) {
			t.Errorf("HoldExpiresAt = %v, want %v", got.HoldExpiresAt, want)
		}
	})

	t.Run("UpdateSeat leaves the store untouched when the mutation fails", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))
		wantErr := errors.New("mutation refused")

		err := repo.UpdateSeat(t.Context(), eventID, "A1", func(seat *domain.Seat) error {
			if err := seat.Hold(testHoldID, testUser, holdTTL, testTime()); err != nil {
				return err
			}

			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("UpdateSeat() error = %v, want %v", err, wantErr)
		}

		got, err := repo.GetSeat(t.Context(), eventID, "A1")
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.Status != domain.StatusAvailable {
			t.Errorf("stored status = %v, want %v", got.Status, domain.StatusAvailable)
		}
		if got.HoldID != "" || got.HeldBy != "" {
			t.Errorf("hold data survived the rollback: %+v", got)
		}
	})

	t.Run("UpdateSeat on an unknown seat returns ErrSeatNotFound", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		err := repo.UpdateSeat(t.Context(), eventID, "Z9", func(*domain.Seat) error {
			t.Error("mutate must not be called for a missing seat")

			return nil
		})
		if !errors.Is(err, ErrSeatNotFound) {
			t.Errorf("UpdateSeat() error = %v, want %v", err, ErrSeatNotFound)
		}
	})

	t.Run("UpdateSeatByHoldID finds the seat carrying the hold", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))
		holdID := eventID + "-hold"

		seedHold(t, repo, eventID, "A1", holdID, testUser, holdTTL)

		err := repo.UpdateSeatByHoldID(t.Context(), holdID, func(seat *domain.Seat) error {
			return seat.Confirm(testUser, testTime().Add(time.Minute))
		})
		if err != nil {
			t.Fatalf("UpdateSeatByHoldID() error = %v", err)
		}

		got, err := repo.GetSeat(t.Context(), eventID, "A1")
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.Status != domain.StatusReserved || got.ReservedBy != testUser {
			t.Errorf("seat = %+v, want reserved by %q", got, testUser)
		}
		if got.HoldID != "" {
			t.Errorf("hold data survived the confirm: %+v", got)
		}
	})

	t.Run("UpdateSeatByHoldID rolls back a failed mutation", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))
		holdID := eventID + "-hold"
		wantErr := errors.New("mutation refused")

		seedHold(t, repo, eventID, "A1", holdID, testUser, holdTTL)

		err := repo.UpdateSeatByHoldID(t.Context(), holdID, func(seat *domain.Seat) error {
			if err := seat.Confirm(testUser, testTime()); err != nil {
				return err
			}

			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("UpdateSeatByHoldID() error = %v, want %v", err, wantErr)
		}

		got, err := repo.GetSeat(t.Context(), eventID, "A1")
		if err != nil {
			t.Fatalf("GetSeat() error = %v", err)
		}
		if got.Status != domain.StatusHeld {
			t.Errorf("stored status = %v, want %v", got.Status, domain.StatusHeld)
		}
	})

	t.Run("UpdateSeatByHoldID on an unknown hold returns ErrHoldNotFound", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))

		err := repo.UpdateSeatByHoldID(t.Context(), "no-such-hold", func(*domain.Seat) error {
			t.Error("mutate must not be called for a missing hold")

			return nil
		})
		if !errors.Is(err, ErrHoldNotFound) {
			t.Errorf("UpdateSeatByHoldID() error = %v, want %v", err, ErrHoldNotFound)
		}
	})

	t.Run("ExpireHolds frees only the holds that have run out", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID,
			domain.NewSeat(eventID, "A1", "A", 1),
			domain.NewSeat(eventID, "A2", "A", 2),
			domain.NewSeat(eventID, "A3", "A", 3),
		)
		now := testTime()

		seedHold(t, repo, eventID, "A1", eventID+"-short", testUser, time.Minute)
		seedHold(t, repo, eventID, "A2", eventID+"-long", testUser, time.Hour)

		expired, err := repo.ExpireHolds(t.Context(), now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("ExpireHolds() error = %v", err)
		}
		if expired != 1 {
			t.Errorf("expired = %d, want 1", expired)
		}

		wantStatus := map[string]domain.SeatStatus{
			"A1": domain.StatusAvailable,
			"A2": domain.StatusHeld,
			"A3": domain.StatusAvailable,
		}

		for seatID, want := range wantStatus {
			got, err := repo.GetSeat(t.Context(), eventID, seatID)
			if err != nil {
				t.Fatalf("GetSeat(%s) error = %v", seatID, err)
			}
			if got.Status != want {
				t.Errorf("%s status = %v, want %v", seatID, got.Status, want)
			}
		}
	})

	t.Run("ExpireHolds treats the expiry instant as expired", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))
		now := testTime()

		seedHold(t, repo, eventID, "A1", eventID+"-hold", testUser, holdTTL)

		expired, err := repo.ExpireHolds(t.Context(), now.Add(holdTTL))
		if err != nil {
			t.Fatalf("ExpireHolds() error = %v", err)
		}
		if expired != 1 {
			t.Errorf("expired = %d, want 1", expired)
		}
	})

	t.Run("ListSeats orders by row then number", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID,
			domain.NewSeat(eventID, "B2", "B", 2),
			domain.NewSeat(eventID, "A10", "A", 10),
			domain.NewSeat(eventID, "A2", "A", 2),
		)

		seats, err := repo.ListSeats(t.Context(), eventID)
		if err != nil {
			t.Fatalf("ListSeats() error = %v", err)
		}

		want := []string{"A2", "A10", "B2"}
		if len(seats) != len(want) {
			t.Fatalf("got %d seats, want %d", len(seats), len(want))
		}

		for i, seat := range seats {
			if seat.ID != want[i] {
				t.Errorf("seat %d = %q, want %q", i, seat.ID, want[i])
			}
		}
	})

	t.Run("ListSeats of an unknown event is empty, not nil", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		seats, err := repo.ListSeats(t.Context(), "no-such-event")
		if err != nil {
			t.Fatalf("ListSeats() error = %v", err)
		}
		if seats == nil {
			t.Error("ListSeats() = nil, want an empty slice")
		}
		if len(seats) != 0 {
			t.Errorf("got %d seats, want 0", len(seats))
		}
	})

	t.Run("only one of many simultaneous holds succeeds", func(t *testing.T) {
		const attempts = 50

		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID, domain.NewSeat(eventID, "A1", "A", 1))
		now := testTime()
		ctx := t.Context()

		var (
			wg        sync.WaitGroup
			succeeded atomic.Int32
			refused   atomic.Int32
		)

		start := make(chan struct{})

		for i := range attempts {
			wg.Add(1)

			go func() {
				defer wg.Done()

				<-start

				err := repo.UpdateSeat(ctx, eventID, "A1", func(seat *domain.Seat) error {
					return seat.Hold(
						fmt.Sprintf("%s-hold-%d", eventID, i),
						fmt.Sprintf("user-%d", i),
						holdTTL,
						now,
					)
				})

				switch {
				case err == nil:
					succeeded.Add(1)
				case errors.Is(err, domain.ErrSeatNotAvailable):
					refused.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}

		close(start)
		wg.Wait()

		if got := succeeded.Load(); got != 1 {
			t.Errorf("successful holds = %d, want exactly 1", got)
		}

		if got := refused.Load(); got != attempts-1 {
			t.Errorf("refused holds = %d, want %d", got, attempts-1)
		}
	})
}

func seedHold(t *testing.T, repo SeatRepository, eventID, seatID, holdID, userID string, ttl time.Duration) {
	t.Helper()

	err := repo.UpdateSeat(t.Context(), eventID, seatID, func(seat *domain.Seat) error {
		return seat.Hold(holdID, userID, ttl, testTime())
	})
	if err != nil {
		t.Fatalf("seeding a hold on %s failed: %v", seatID, err)
	}
}

var (
	_ SeatRepository  = (*MemorySeatRepository)(nil)
	_ SeatRepository  = (*PostgresSeatRepository)(nil)
	_ SeatRepository  = (*OptimisticSeatRepository)(nil)
	_ EventRepository = (*MemoryEventRepository)(nil)
	_ EventRepository = (*PostgresEventRepository)(nil)
)
