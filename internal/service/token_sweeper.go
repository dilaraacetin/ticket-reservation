package service

import (
	"context"
	"log/slog"
	"time"
)

// DefaultRevocationSweepInterval is how often spent revocations are cleared.
//
// Unhurried on purpose. A record that has outlived its token refuses nothing the
// expiry check does not already refuse, so leaving one for a few minutes costs
// a row rather than correctness.
const DefaultRevocationSweepInterval = 5 * time.Minute

// Revoker is the slice of the account service this sweeper needs.
type Revoker interface {
	SweepRevocations(ctx context.Context, now time.Time) (int, error)
}

// TokenSweeper clears revocations whose tokens have expired. Without it the
// denylist only grows, and it is the one table here that would grow for no
// reason at all.
type TokenSweeper struct {
	accounts Revoker
	clock    Clock
	interval time.Duration
	logger   *slog.Logger
}

func NewTokenSweeper(accounts Revoker, clock Clock, interval time.Duration, logger *slog.Logger) *TokenSweeper {
	if interval <= 0 {
		interval = DefaultRevocationSweepInterval
	}

	return &TokenSweeper{accounts: accounts, clock: clock, interval: interval, logger: logger}
}

// Run sweeps on every tick until ctx is cancelled. It blocks, so callers start
// it in its own goroutine.
func (w *TokenSweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.InfoContext(ctx, "revocation sweeper started", "interval", w.interval)

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "revocation sweeper stopped")

			return nil
		case <-ticker.C:
			w.Sweep(ctx)
		}
	}
}

// Sweep runs one pass and reports how many records it cleared. A failed pass is
// logged rather than returned, because one bad pass must not take the worker
// down; the next tick will try again.
func (w *TokenSweeper) Sweep(ctx context.Context) int {
	cleared, err := w.accounts.SweepRevocations(ctx, w.clock.Now())
	if err != nil {
		w.logger.ErrorContext(ctx, "clearing spent revocations failed", "err", err)

		return 0
	}

	if cleared > 0 {
		w.logger.InfoContext(ctx, "cleared spent revocations", "count", cleared)
	}

	return cleared
}
