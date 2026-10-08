package service

import (
	"context"
	"log/slog"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// DefaultSweepInterval is how often expired holds are swept up. It only bounds
// how long a freed seat stays invisible in a seat map, because Seat.CanHold
// already treats an expired hold as gone.
const DefaultSweepInterval = 30 * time.Second

// HoldSweeper releases expired holds in the background. Without it a seat whose
// hold ran out would keep reporting itself as held until somebody tried to take
// it, which would make seat maps lie.
type HoldSweeper struct {
	seats    repository.SeatRepository
	clock    Clock
	interval time.Duration
	logger   *slog.Logger
	offerer  SeatOfferer
}

func NewHoldSweeper(
	seats repository.SeatRepository,
	clock Clock,
	interval time.Duration,
	logger *slog.Logger,
) *HoldSweeper {
	return &HoldSweeper{
		seats:    seats,
		clock:    clock,
		interval: interval,
		logger:   logger,
		offerer:  discardOfferer{},
	}
}

// WithOfferer wires in what happens to the seats a sweep frees.
func (w *HoldSweeper) WithOfferer(offerer SeatOfferer) *HoldSweeper {
	w.offerer = offerer

	return w
}

// Run sweeps on every tick until ctx is cancelled, then returns nil. It blocks,
// so callers start it in its own goroutine and cancel the context to shut it
// down.
func (w *HoldSweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.InfoContext(ctx, "hold sweeper started", "interval", w.interval)

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "hold sweeper stopped")

			return nil
		case <-ticker.C:
			w.Sweep(ctx)
		}
	}
}

// Sweep runs one pass and returns the seats it freed. A failed sweep is logged
// rather than returned, because one bad pass must not take the worker down; the
// next tick will try again.
func (w *HoldSweeper) Sweep(ctx context.Context) []domain.SeatRef {
	freed, err := w.seats.ExpireHolds(ctx, w.clock.Now())
	if err != nil {
		w.logger.ErrorContext(ctx, "sweeping expired holds failed", "err", err)

		return nil
	}

	if len(freed) > 0 {
		w.logger.InfoContext(ctx, "released expired holds", "count", len(freed))
	}

	for _, seat := range freed {
		w.offerer.Offer(seat)
	}

	return freed
}
