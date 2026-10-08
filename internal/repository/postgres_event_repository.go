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

// PostgresEventRepository stores events in PostgreSQL.
type PostgresEventRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresEventRepository returns a store over the given connection pool.
func NewPostgresEventRepository(pool *pgxpool.Pool) *PostgresEventRepository {
	return &PostgresEventRepository{pool: pool}
}

const (
	eventColumns = `id, name, venue, starts_at, cancelled_at,
	                city, category, image_url, description, rules`

	selectEventSQL = `select ` + eventColumns + ` from events where id = $1`

	selectEventForUpdateSQL = selectEventSQL + ` for update`

	// The placeholders follow eventValues, with the cancellation stamp appended,
	// so the argument list is one slice rather than a splice.
	updateEventSQL = `update events
		   set name = $2, venue = $3, starts_at = $4,
		       city = $5, category = $6, image_url = $7, description = $8, rules = $9,
		       cancelled_at = $10
		 where id = $1`

	// One statement, so the check and the delete cannot be split apart. A seat
	// that stops being available between the two would otherwise let an event
	// people hold tickets to disappear.
	deleteEventSQL = `delete from events
		 where id = $1
		   and not exists (
		       select 1 from seats
		        where seats.event_id = events.id
		          and seats.status <> 'available'
		   )`

	listEventsSQL = `select ` + eventColumns + ` from events order by starts_at, id`

	insertEventSQL = `insert into events
		     (id, name, venue, starts_at, city, category, image_url, description, rules)
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 on conflict (id) do nothing`
)

// rowScanner is what pgx.Row and pgx.Rows have in common, so a single read of
// an event row serves the one-row queries and the listing alike.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanEvent reads one row of eventColumns.
func scanEvent(row rowScanner) (*domain.Event, error) {
	var (
		event       domain.Event
		cancelledAt *time.Time
	)

	var category string

	err := row.Scan(
		&event.ID, &event.Name, &event.Venue, &event.StartsAt, &cancelledAt,
		&event.Details.City, &category,
		&event.Details.ImageURL, &event.Details.Description, &event.Details.Rules,
	)
	if err != nil {
		return nil, err
	}

	// A row holding something that is not a category has to fail rather than be
	// read as one, the same way a stored role does.
	event.Details.Category, err = domain.ParseCategory(category)
	if err != nil {
		return nil, fmt.Errorf("reading event %s: %w", event.ID, err)
	}

	event.CancelledAt = instant(cancelledAt)

	return &event, nil
}

// eventValues lists an event the way insertEventSQL and updateEventSQL take it.
func eventValues(event *domain.Event) []any {
	details := event.Details.WithDefaults()

	return []any{
		event.ID, event.Name, event.Venue, event.StartsAt,
		details.City, details.Category.String(),
		details.ImageURL, details.Description, details.Rules,
	}
}

// GetEvent reads one event.
func (r *PostgresEventRepository) GetEvent(ctx context.Context, eventID string) (*domain.Event, error) {
	event, err := scanEvent(r.pool.QueryRow(ctx, selectEventSQL, eventID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}

		return nil, fmt.Errorf("reading event %s: %w", eventID, err)
	}

	return event, nil
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
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning an event: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	return events, nil
}

// UpdateEvent locks the row, runs mutate, and writes the result back. The same
// shape as UpdateSeat, for the same reason: reading, deciding and writing have
// to be one step.
func (r *PostgresEventRepository) UpdateEvent(
	ctx context.Context,
	eventID string,
	mutate func(*domain.Event) error,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("updating event %s: %w", eventID, err)
	}

	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	event, err := scanEvent(tx.QueryRow(ctx, selectEventForUpdateSQL, eventID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEventNotFound
		}

		return fmt.Errorf("updating event %s: %w", eventID, err)
	}

	if err := mutate(event); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, updateEventSQL,
		append(eventValues(event), nullInstant(event.CancelledAt))...)
	if err != nil {
		return fmt.Errorf("updating event %s: %w", eventID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("updating event %s: %w", eventID, err)
	}

	return nil
}

// DeleteEvent removes an event unless somebody holds or owns one of its seats.
func (r *PostgresEventRepository) DeleteEvent(ctx context.Context, eventID string) error {
	tag, err := r.pool.Exec(ctx, deleteEventSQL, eventID)
	if err != nil {
		return fmt.Errorf("deleting event %s: %w", eventID, err)
	}

	if tag.RowsAffected() == 0 {
		// Nothing went: either there was no such event, or it has seats that are
		// spoken for. The read that tells them apart is worth it, because the
		// answers mean different things to whoever asked.
		if _, err := r.GetEvent(ctx, eventID); err != nil {
			return err
		}

		return ErrEventInUse
	}

	return nil
}

// InsertEvents seeds events, ignoring ones that already exist.
// CreateEvent stores one event and reports a clash rather than ignoring it, so
// that a request asking for an id that is taken is told instead of appearing to
// have worked.
func (r *PostgresEventRepository) CreateEvent(ctx context.Context, event *domain.Event) error {
	tag, err := r.pool.Exec(ctx, insertEventSQL, eventValues(event)...)
	if err != nil {
		return fmt.Errorf("creating an event: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrEventExists
	}

	return nil
}

func (r *PostgresEventRepository) InsertEvents(ctx context.Context, events ...*domain.Event) error {
	batch := &pgx.Batch{}
	for _, event := range events {
		batch.Queue(insertEventSQL, eventValues(event)...)
	}

	if err := r.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("inserting events: %w", err)
	}

	return nil
}
