package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/domain"
)

type PostgresWaitingListRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresWaitingListRepository(pool *pgxpool.Pool) *PostgresWaitingListRepository {
	return &PostgresWaitingListRepository{pool: pool}
}

const waitingColumns = `id, event_id, user_id, joined_at`

const (
	// The update writes the stored joined_at back over itself. It looks pointless
	// and is deliberate: on conflict do nothing returns no row, and this is what
	// makes RETURNING hand back the place the user already had.
	joinWaitingListSQL = `insert into waiting_list (` + waitingColumns + `)
		 values ($1, $2, $3, $4)
		 on conflict (event_id, user_id)
		 do update set joined_at = waiting_list.joined_at
	  returning ` + waitingColumns

	leaveWaitingListSQL = `delete from waiting_list where event_id = $1 and user_id = $2`

	// skip locked is what lets two workers make progress at once. Without it the
	// second one blocks on the row the first has locked instead of serving the
	// next person in the queue, so a pool of workers would quietly run one at a
	// time.
	takeNextWaitingSQL = `delete from waiting_list
		 where id = (
		       select id
		         from waiting_list
		        where event_id = $1
		        order by joined_at, id
		        limit 1
		          for update skip locked
		 )
	  returning ` + waitingColumns

	// One statement: the rows go and come back in the same breath, so nobody can
	// be taken off the queue between the two and lose their notice.
	drainWaitingListSQL = `delete from waiting_list
		 where event_id = $1
	  returning ` + waitingColumns

	rejoinWaitingListSQL = `insert into waiting_list (` + waitingColumns + `)
		 values ($1, $2, $3, $4)
		 on conflict do nothing`

	// Counting everyone at or ahead of the user gives the one based position
	// directly. An absent user joins against nothing, so the count comes back as
	// zero, which no real position can be.
	waitingPositionSQL = `with me as (
		     select joined_at, id from waiting_list where event_id = $1 and user_id = $2
		 )
		 select count(*)
		   from waiting_list w, me
		  where w.event_id = $1
		    and (w.joined_at, w.id) <= (me.joined_at, me.id)`
)

func scanWaitingEntry(row pgx.Row) (*domain.WaitingEntry, error) {
	var entry domain.WaitingEntry
	if err := row.Scan(&entry.ID, &entry.EventID, &entry.UserID, &entry.JoinedAt); err != nil {
		return nil, err
	}

	return &entry, nil
}

// Join inserts and reads the position back inside one transaction.
//
// Two statements rather than one, because a data modifying CTE and the query
// that reads it see the same snapshot: a count in the same statement would not
// see the row just inserted. Inside a transaction the insert is visible to the
// count that follows it, and nothing else can take the caller off the queue in
// between.
func (r *PostgresWaitingListRepository) Join(
	ctx context.Context,
	entry *domain.WaitingEntry,
) (*domain.WaitingEntry, int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("joining the waiting list: %w", err)
	}

	defer func() {
		// After a commit this is a no-op; it is here for every path that is not
		// one, so a failed join does not leave the connection in a transaction.
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()

	joined, err := scanWaitingEntry(
		tx.QueryRow(ctx, joinWaitingListSQL, entry.ID, entry.EventID, entry.UserID, entry.JoinedAt))
	if err != nil {
		return nil, 0, fmt.Errorf("joining the waiting list: %w", err)
	}

	var position int
	if err := tx.QueryRow(ctx, waitingPositionSQL, entry.EventID, entry.UserID).Scan(&position); err != nil {
		return nil, 0, fmt.Errorf("joining the waiting list: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, fmt.Errorf("joining the waiting list: %w", err)
	}

	return joined, position, nil
}

func (r *PostgresWaitingListRepository) Leave(ctx context.Context, eventID, userID string) error {
	if _, err := r.pool.Exec(ctx, leaveWaitingListSQL, eventID, userID); err != nil {
		return fmt.Errorf("leaving the waiting list: %w", err)
	}

	return nil
}

func (r *PostgresWaitingListRepository) TakeNext(
	ctx context.Context,
	eventID string,
) (*domain.WaitingEntry, error) {
	taken, err := scanWaitingEntry(r.pool.QueryRow(ctx, takeNextWaitingSQL, eventID))

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrWaitingListEmpty
	case err != nil:
		return nil, fmt.Errorf("taking the next waiter: %w", err)
	}

	return taken, nil
}

// Drain empties the queue and returns who was in it.
func (r *PostgresWaitingListRepository) Drain(
	ctx context.Context,
	eventID string,
) ([]*domain.WaitingEntry, error) {
	rows, err := r.pool.Query(ctx, drainWaitingListSQL, eventID)
	if err != nil {
		return nil, fmt.Errorf("draining the waiting list: %w", err)
	}
	defer rows.Close()

	drained := make([]*domain.WaitingEntry, 0)

	for rows.Next() {
		entry, err := scanWaitingEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("draining the waiting list: %w", err)
		}

		drained = append(drained, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("draining the waiting list: %w", err)
	}

	return drained, nil
}

func (r *PostgresWaitingListRepository) Rejoin(ctx context.Context, entry *domain.WaitingEntry) error {
	_, err := r.pool.Exec(ctx, rejoinWaitingListSQL, entry.ID, entry.EventID, entry.UserID, entry.JoinedAt)
	if err != nil {
		return fmt.Errorf("rejoining the waiting list: %w", err)
	}

	return nil
}

func (r *PostgresWaitingListRepository) Position(ctx context.Context, eventID, userID string) (int, error) {
	var position int
	if err := r.pool.QueryRow(ctx, waitingPositionSQL, eventID, userID).Scan(&position); err != nil {
		return 0, fmt.Errorf("reading the waiting list position: %w", err)
	}

	if position == 0 {
		return 0, ErrNotWaiting
	}

	return position, nil
}

var _ WaitingListRepository = (*PostgresWaitingListRepository)(nil)

var _ WaitingListRepository = (*MemoryWaitingListRepository)(nil)
