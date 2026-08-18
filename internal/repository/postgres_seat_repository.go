package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/domain"
)

type PostgresSeatRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresSeatRepository(pool *pgxpool.Pool) *PostgresSeatRepository {
	return &PostgresSeatRepository{pool: pool}
}

const (
	selectSeatSQL = `select ` + seatColumns + ` from seats where event_id = $1 and id = $2`

	selectSeatForUpdateSQL = selectSeatSQL + ` for update`

	selectSeatByHoldForUpdateSQL = `select ` + seatColumns + `
		  from seats
		 where hold_id = $1
		   for update`

	listSeatsSQL = `select ` + seatColumns + `
		  from seats
		 where event_id = $1
		 order by row_label, number`

	updateSeatSQL = `update seats
		   set status = $3,
		       hold_id = $4,
		       held_by = $5,
		       hold_created_at = $6,
		       hold_expires_at = $7,
		       reserved_by = $8,
		       version = version + 1
		 where event_id = $1
		   and id = $2`

	expireHoldsSQL = `update seats
		   set status = 'available',
		       hold_id = null,
		       held_by = null,
		       hold_created_at = null,
		       hold_expires_at = null,
		       version = version + 1
		 where status = 'held'
		   and hold_expires_at <= $1`

	insertSeatSQL = `insert into seats (event_id, id, row_label, number, status)
		 values ($1, $2, $3, $4, $5)
		 on conflict (event_id, id) do nothing`
)

// GetSeat reads one seat.
func (r *PostgresSeatRepository) GetSeat(ctx context.Context, eventID, seatID string) (*domain.Seat, error) {
	var row seatRow

	if err := row.scan(r.pool.QueryRow(ctx, selectSeatSQL, eventID, seatID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSeatNotFound
		}

		return nil, fmt.Errorf("reading seat %s/%s: %w", eventID, seatID, err)
	}

	return row.toDomain()
}

// ListSeats reads every seat of an event, ordered by row and number.
func (r *PostgresSeatRepository) ListSeats(ctx context.Context, eventID string) ([]*domain.Seat, error) {
	return listSeats(ctx, r.pool, eventID)
}

// listSeats and insertSeats are shared with the optimistic store: reads and seeds
// need no concurrency strategy, so only the update path differs between the two.
func listSeats(ctx context.Context, pool *pgxpool.Pool, eventID string) ([]*domain.Seat, error) {
	rows, err := pool.Query(ctx, listSeatsSQL, eventID)
	if err != nil {
		return nil, fmt.Errorf("listing seats of %s: %w", eventID, err)
	}
	defer rows.Close()

	seats := make([]*domain.Seat, 0)

	for rows.Next() {
		var row seatRow
		if err := row.scan(rows); err != nil {
			return nil, fmt.Errorf("scanning a seat of %s: %w", eventID, err)
		}

		seat, err := row.toDomain()
		if err != nil {
			return nil, err
		}

		seats = append(seats, seat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing seats of %s: %w", eventID, err)
	}

	return seats, nil
}

func (r *PostgresSeatRepository) UpdateSeat(
	ctx context.Context,
	eventID, seatID string,
	mutate func(*domain.Seat) error,
) error {
	return r.inTransaction(ctx, func(tx pgx.Tx) error {
		var row seatRow

		if err := row.scan(tx.QueryRow(ctx, selectSeatForUpdateSQL, eventID, seatID)); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrSeatNotFound
			}

			return fmt.Errorf("locking seat %s/%s: %w", eventID, seatID, err)
		}

		return applyMutation(ctx, tx, row, mutate)
	})
}

// UpdateSeatByHoldID is UpdateSeat for callers that only know a hold id. The
// partial unique index on hold_id makes this an indexed lookup rather than the
// scan the in-memory store performs.
func (r *PostgresSeatRepository) UpdateSeatByHoldID(
	ctx context.Context,
	holdID string,
	mutate func(*domain.Seat) error,
) error {
	return r.inTransaction(ctx, func(tx pgx.Tx) error {
		var row seatRow

		if err := row.scan(tx.QueryRow(ctx, selectSeatByHoldForUpdateSQL, holdID)); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrHoldNotFound
			}

			return fmt.Errorf("locking the seat of hold %s: %w", holdID, err)
		}

		return applyMutation(ctx, tx, row, mutate)
	})
}

// ExpireHolds frees every expired hold in one statement.
func (r *PostgresSeatRepository) ExpireHolds(ctx context.Context, now time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, expireHoldsSQL, now)
	if err != nil {
		return 0, fmt.Errorf("expiring holds: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

// InsertSeats seeds seats. It is not part of SeatRepository because creating the
// seat map is an administrative act, not part of the reservation flow.
func (r *PostgresSeatRepository) InsertSeats(ctx context.Context, seats ...*domain.Seat) error {
	return insertSeats(ctx, r.pool, seats...)
}

func insertSeats(ctx context.Context, pool *pgxpool.Pool, seats ...*domain.Seat) error {
	if len(seats) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, seat := range seats {
		batch.Queue(insertSeatSQL, seat.EventID, seat.ID, seat.Row, seat.Number, seat.Status.String())
	}

	if err := pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("inserting seats: %w", err)
	}

	return nil
}

// applyMutation runs mutate on the loaded row and persists the result.
func applyMutation(ctx context.Context, tx pgx.Tx, row seatRow, mutate func(*domain.Seat) error) error {
	seat, err := row.toDomain()
	if err != nil {
		return err
	}

	if err := mutate(seat); err != nil {
		return err
	}

	return writeSeat(ctx, tx, seat)
}

func writeSeat(ctx context.Context, tx pgx.Tx, seat *domain.Seat) error {
	_, err := tx.Exec(ctx, updateSeatSQL,
		seat.EventID,
		seat.ID,
		seat.Status.String(),
		nullText(seat.HoldID),
		nullText(seat.HeldBy),
		nullInstant(seat.HoldCreatedAt),
		nullInstant(seat.HoldExpiresAt),
		nullText(seat.ReservedBy),
	)
	if err != nil {
		return fmt.Errorf("writing seat %s/%s: %w", seat.EventID, seat.ID, err)
	}

	return nil
}

// inTransaction runs fn in a transaction, committing on success and rolling back
// on any error.
func (r *PostgresSeatRepository) inTransaction(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning a transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}
