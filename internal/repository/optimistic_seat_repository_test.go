package repository

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"ticket-reservation/internal/domain"
)

// TestOptimisticSeatRepository_LostRacesAreRetried shows what the retry loop buys:
// callers see the ordinary domain refusal, never the fact that a write was lost
// and repeated. The conflict count is logged rather than asserted, because how
// many races actually occur depends on the machine.
func TestOptimisticSeatRepository_LostRacesAreRetried(t *testing.T) {
	const attempts = 50

	eventID := uniqueEventID(t)
	pool := seededPool(t, eventID)
	repo := NewOptimisticSeatRepository(pool, DefaultUpdateAttempts)

	if err := repo.InsertSeats(t.Context(), domain.NewSeat(eventID, "A1", "A", 1)); err != nil {
		t.Fatalf("seeding the seat failed: %v", err)
	}

	var (
		wg        sync.WaitGroup
		succeeded atomic.Int32
		refused   atomic.Int32
		gaveUp    atomic.Int32
	)

	ctx := t.Context()
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
					testTime(),
				)
			})

			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, domain.ErrSeatNotAvailable):
				refused.Add(1)
			case errors.Is(err, ErrConcurrentUpdate):
				gaveUp.Add(1)
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

	if got := gaveUp.Load(); got != 0 {
		t.Errorf("%d callers exhausted their retries, want 0", got)
	}

	t.Logf("conflicts retried: %d out of %d attempts", repo.Conflicts(), attempts)
}

func TestOptimisticSeatRepository_GivesUpAfterTooManyConflicts(t *testing.T) {
	eventID := uniqueEventID(t)
	pool := seededPool(t, eventID)

	repo := NewOptimisticSeatRepository(pool, 1)

	if err := repo.InsertSeats(t.Context(), domain.NewSeat(eventID, "A1", "A", 1)); err != nil {
		t.Fatalf("seeding the seat failed: %v", err)
	}

	ctx := t.Context()

	err := repo.UpdateSeat(ctx, eventID, "A1", func(seat *domain.Seat) error {
		bump := `update seats set version = version + 1 where event_id = $1 and id = $2`
		if _, err := pool.Exec(ctx, bump, eventID, "A1"); err != nil {
			return fmt.Errorf("simulating a competing writer: %w", err)
		}

		return seat.Hold(testHoldID, testUser, holdTTL, testTime())
	})
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("UpdateSeat() error = %v, want %v", err, ErrConcurrentUpdate)
	}

	got, err := repo.GetSeat(ctx, eventID, "A1")
	if err != nil {
		t.Fatalf("GetSeat() error = %v", err)
	}
	if got.Status != domain.StatusAvailable {
		t.Errorf("status = %v, want %v", got.Status, domain.StatusAvailable)
	}
	if got.HoldID != "" {
		t.Errorf("hold data was written despite the conflict: %+v", got)
	}
}

func TestOptimisticSeatRepository_DefaultsTheAttemptBudget(t *testing.T) {
	for _, attempts := range []int{0, -1} {
		repo := NewOptimisticSeatRepository(nil, attempts)
		if repo.attempts != DefaultUpdateAttempts {
			t.Errorf("attempts for %d = %d, want %d", attempts, repo.attempts, DefaultUpdateAttempts)
		}
	}
}
