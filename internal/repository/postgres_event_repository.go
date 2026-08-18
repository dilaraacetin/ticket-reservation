package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/domain"
)

// PostgresEventRepository stores events in PostgreSQL.
type PostgresEventRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresEventRepository returns a store over the given connection pool.
func NewPostgresEventRepository(pool *pgxpool.Pool) *PostgresEventRepository {
	return &PostgresEventRepository{pool: pool}
}

const (
	eventColumns = `id, name, venue, starts_at`

	selectEventSQL = `select ` + eventColumns + ` from events where id = $1`

	listEventsSQL = `select ` + eventColumns + ` from events order by starts_at, id`

	insertEventSQL = `insert into events (id, name, venue, starts_at)
		 values ($1, $2, $3, $4)
		 on conflict (id) do nothing`
)

// GetEvent reads one event.
func (r *PostgresEventRepository) GetEvent(ctx context.Context, eventID string) (*domain.Event, error) {
	var event domain.Event

	err := r.pool.QueryRow(ctx, selectEventSQL, eventID).
		Scan(&event.ID, &event.Name, &event.Venue, &event.StartsAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}

		return nil, fmt.Errorf("reading event %s: %w", eventID, err)
	}

	return &event, nil
}

// ListEvents reads every event, soonest first.
func (r *PostgresEventRepository) ListEvents(ctx context.Context) ([]*domain.Event, error) {
	rows, err := r.pool.Query(ctx, listEventsSQL)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	defer rows.Close()

	events := make([]*domain.Event, 0)

	for rows.Next() {
		var event domain.Event
		if err := rows.Scan(&event.ID, &event.Name, &event.Venue, &event.StartsAt); err != nil {
			return nil, fmt.Errorf("scanning an event: %w", err)
		}

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	return events, nil
}

// InsertEvents seeds events, ignoring ones that already exist.
func (r *PostgresEventRepository) InsertEvents(ctx context.Context, events ...*domain.Event) error {
	batch := &pgx.Batch{}
	for _, event := range events {
		batch.Queue(insertEventSQL, event.ID, event.Name, event.Venue, event.StartsAt)
	}

	if err := r.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("inserting events: %w", err)
	}

	return nil
}
