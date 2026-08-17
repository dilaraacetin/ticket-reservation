package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

type failingSeatRepository struct {
	err error
}

func (r failingSeatRepository) GetSeat(context.Context, string, string) (*domain.Seat, error) {
	return nil, r.err
}

func (r failingSeatRepository) ListSeats(context.Context, string) ([]*domain.Seat, error) {
	return nil, r.err
}

func (r failingSeatRepository) UpdateSeat(
	context.Context, string, string, func(*domain.Seat) error,
) error {
	return r.err
}

func (r failingSeatRepository) UpdateSeatByHoldID(
	context.Context, string, func(*domain.Seat) error,
) error {
	return r.err
}

func (r failingSeatRepository) ExpireHolds(context.Context, time.Time) (int, error) {
	return 0, r.err
}

func newTestSweeper(interval time.Duration) (*HoldSweeper, *fakeClock, *ReservationService) {
	clock := newFakeClock(testTime())
	repo := repository.NewMemorySeatRepository(
		domain.NewSeat(testEventID, "A1", "A", 1),
		domain.NewSeat(testEventID, "A2", "A", 2),
	)
	svc := NewReservationService(repo, clock, NewRandomID, DefaultHoldTTL)

	return NewHoldSweeper(repo, clock, interval, discardLogger()), clock, svc
}

func TestHoldSweeper_Sweep(t *testing.T) {
	ctx := context.Background()

	t.Run("releases only the holds that have run out", func(t *testing.T) {
		sweeper, clock, svc := newTestSweeper(DefaultSweepInterval)

		for _, seatID := range []string{"A1", "A2"} {
			if _, err := svc.HoldSeat(ctx, testEventID, seatID, testUser); err != nil {
				t.Fatalf("HoldSeat(%s) error = %v", seatID, err)
			}
		}

		if got := sweeper.Sweep(ctx); got != 0 {
			t.Errorf("Sweep() before expiry = %d, want 0", got)
		}

		clock.Advance(DefaultHoldTTL)

		if got := sweeper.Sweep(ctx); got != 2 {
			t.Errorf("Sweep() after expiry = %d, want 2", got)
		}

		// Nothing is left to release, so a second pass is a no-op.
		if got := sweeper.Sweep(ctx); got != 0 {
			t.Errorf("second Sweep() = %d, want 0", got)
		}
	})

	t.Run("a failing store is reported as zero rather than panicking", func(t *testing.T) {
		sweeper := NewHoldSweeper(
			failingSeatRepository{err: errors.New("store is unreachable")},
			newFakeClock(testTime()),
			DefaultSweepInterval,
			discardLogger(),
		)

		if got := sweeper.Sweep(ctx); got != 0 {
			t.Errorf("Sweep() = %d, want 0", got)
		}
	})

	t.Run("an empty store sweeps nothing", func(t *testing.T) {
		sweeper := NewHoldSweeper(
			repository.NewMemorySeatRepository(),
			newFakeClock(testTime()),
			DefaultSweepInterval,
			discardLogger(),
		)

		if got := sweeper.Sweep(ctx); got != 0 {
			t.Errorf("Sweep() = %d, want 0", got)
		}
	})
}
func TestHoldSweeper_Run_StopsOnCancel(t *testing.T) {
	sweeper, _, _ := newTestSweeper(time.Hour) // long enough that no tick fires

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- sweeper.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() error = %v, want nil after cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not return within a second of cancellation")
	}
}

func TestHoldSweeper_Run_SweepsOnTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sweeper, clock, svc := newTestSweeper(time.Millisecond)

	if _, err := svc.HoldSeat(ctx, testEventID, "A1", testUser); err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	clock.Advance(DefaultHoldTTL)

	done := make(chan error, 1)
	go func() {
		done <- sweeper.Run(ctx)
	}()

	deadline := time.After(2 * time.Second)

	for {
		seat, err := svc.Seat(ctx, testEventID, "A1")
		if err != nil {
			t.Fatalf("Seat() error = %v", err)
		}
		if seat.Status == domain.StatusAvailable {
			break
		}

		select {
		case <-deadline:
			t.Fatal("the sweeper did not release the expired hold")
		default:
		}
	}

	cancel()

	if err := <-done; err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
}
