package repository

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/domain"
)

const DefaultUpdateAttempts = 5

type OptimisticSeatRepository struct {
	pool      *pgxpool.Pool
	attempts  int
	conflicts atomic.Int64
}

// NewOptimisticSeatRepository returns a store that resolves conflicts by retrying.
func NewOptimisticSeatRepository(pool *pgxpool.Pool, attempts int) *OptimisticSeatRepository {
	if attempts <= 0 {
		attempts = DefaultUpdateAttempts
	}

	return &OptimisticSeatRepository{pool: pool, attempts: attempts}
}

// Conflicts reports how many attempts have been lost and retried so far.
func (r *OptimisticSeatRepository) Conflicts() int64 {
	return r.conflicts.Load()
}

const (
	selectSeatByHoldSQL = `select ` + seatColumns + ` from seats where hold_id = $1`

	updateSeatIfUnchangedSQL = `update seats
		   set status = $3,
		       hold_id = $4,
		       held_by = $5,
		       hold_created_at = $6,
		       hold_expires_at = $7,
		       reserved_by = $8,
		       version = version + 1
		 where event_id = $1
		   and id = $2
		   and version = $9`
)

// GetSeat reads one seat. Reads need no strategy at all, so this is identical to
// the pessimistic store.
func (r *OptimisticSeatRepository) GetSeat(ctx context.Context, eventID, seatID string) (*domain.Seat, error) {
	var row seatRow

	if err := row.scan(r.pool.QueryRow(ctx, selectSeatSQL, eventID, seatID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSeatNotFound
		}

		return nil, fmt.Errorf("reading seat %s/%s: %w", eventID, seatID, err)
	}

	return row.toDomain()
}

// ListSeats reads every seat of an event.
func (r *OptimisticSeatRepository) ListSeats(ctx context.Context, eventID string) ([]*domain.Seat, error) {
	return listSeats(ctx, r.pool, eventID)
}

// UpdateSeat reads the seat, applies mutate and writes the result back only if
// nobody else has written in the meantime. A lost race is retried from the read,
// which is what makes the caller see the same outcome as with row locking: on the
// second read the seat is held, and the domain refuses the hold on its own terms.
func (r *OptimisticSeatRepository) UpdateSeat(
	ctx context.Context,
	eventID, seatID string,
	mutate func(*domain.Seat) error,
) error {
	return r.withRetries(ctx, func() (bool, error) {
		var row seatRow

		if err := row.scan(r.pool.QueryRow(ctx, selectSeatSQL, eventID, seatID)); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, ErrSeatNotFound
			}

			return false, fmt.Errorf("reading seat %s/%s: %w", eventID, seatID, err)
		}

		return r.applyIfUnchanged(ctx, row, mutate)
	})
}

// UpdateSeatByHoldID is UpdateSeat for callers that only know a hold id.
func (r *OptimisticSeatRepository) UpdateSeatByHoldID(
	ctx context.Context,
	holdID string,
	mutate func(*domain.Seat) error,
) error {
	return r.withRetries(ctx, func() (bool, error) {
		var row seatRow

		if err := row.scan(r.pool.QueryRow(ctx, selectSeatByHoldSQL, holdID)); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, ErrHoldNotFound
			}

			return false, fmt.Errorf("reading the seat of hold %s: %w", holdID, err)
		}

		return r.applyIfUnchanged(ctx, row, mutate)
	})
}

// ExpireHolds is unchanged from the pessimistic store: a single statement is
// already atomic, so there is no read-then-write gap for a version to guard.
func (r *OptimisticSeatRepository) ExpireHolds(ctx context.Context, now time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, expireHoldsSQL, now)
	if err != nil {
		return 0, fmt.Errorf("expiring holds: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

// InsertSeats seeds seats.
func (r *OptimisticSeatRepository) InsertSeats(ctx context.Context, seats ...*domain.Seat) error {
	return insertSeats(ctx, r.pool, seats...)
}

// applyIfUnchanged reports whether the attempt should be repeated.
func (r *OptimisticSeatRepository) applyIfUnchanged(
	ctx context.Context,
	row seatRow,
	mutate func(*domain.Seat) error,
) (bool, error) {
	seat, err := row.toDomain()
	if err != nil {
		return false, err
	}

	if err := mutate(seat); err != nil {
		return false, err
	}

	tag, err := r.pool.Exec(ctx, updateSeatIfUnchangedSQL,
		seat.EventID,
		seat.ID,
		seat.Status.String(),
		nullText(seat.HoldID),
		nullText(seat.HeldBy),
		nullInstant(seat.HoldCreatedAt),
		nullInstant(seat.HoldExpiresAt),
		nullText(seat.ReservedBy),
		row.version,
	)
	if err != nil {
		return false, fmt.Errorf("writing seat %s/%s: %w", seat.EventID, seat.ID, err)
	}

	// Zero rows means the version moved under us. The row may also have vanished,
	// but there is no need to tell the two apart here: the retry's read reports
	// ErrSeatNotFound if it is really gone.
	return tag.RowsAffected() == 0, nil
}

// withRetries runs attempt until it succeeds, fails outright, or runs out of
// tries. A lost race is not an error the caller should see, so it is not returned
// unless every attempt is used up.
func (r *OptimisticSeatRepository) withRetries(ctx context.Context, attempt func() (bool, error)) error {
	for range r.attempts {
		// Checked before each try, so a cancelled request stops retrying instead
		// of hammering the database on its way out.
		if err := ctx.Err(); err != nil {
			return err
		}

		retry, err := attempt()
		if err != nil {
			return err
		}
		if !retry {
			return nil
		}

		r.conflicts.Add(1)
	}

	return fmt.Errorf("%w after %d attempts", ErrConcurrentUpdate, r.attempts)
}
