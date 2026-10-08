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

type PostgresNotificationRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresNotificationRepository(pool *pgxpool.Pool) *PostgresNotificationRepository {
	return &PostgresNotificationRepository{pool: pool}
}

const notificationColumns = `id, user_id, kind, event_id, event_name, seat_id, created_at, read_at`

const (
	insertNotificationSQL = `insert into notifications (` + notificationColumns + `)
		 values ($1, $2, $3, $4, $5, $6, $7, null)
		 on conflict (id) do nothing`

	getNotificationSQL = `select ` + notificationColumns + ` from notifications where id = $1`

	listNotificationsSQL = `select ` + notificationColumns + `
		  from notifications
		 where user_id = $1
		 order by created_at desc, id`

	markAllReadSQL = `update notifications
		   set read_at = $2
		 where user_id = $1 and read_at is null`
)

// Notify writes the notices in one batch, because they are produced together.
func (r *PostgresNotificationRepository) Notify(
	ctx context.Context,
	notifications ...*domain.Notification,
) error {
	if len(notifications) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, notification := range notifications {
		batch.Queue(insertNotificationSQL,
			notification.ID, notification.UserID, notification.Kind.String(),
			notification.EventID, notification.EventName, notification.SeatID,
			notification.CreatedAt)
	}

	if err := r.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("storing notifications: %w", err)
	}

	return nil
}

// Get returns one notice.
func (r *PostgresNotificationRepository) Get(
	ctx context.Context,
	notificationID string,
) (*domain.Notification, error) {
	notification, err := scanNotification(r.pool.QueryRow(ctx, getNotificationSQL, notificationID))

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotificationNotFound
	case err != nil:
		return nil, fmt.Errorf("reading notification %s: %w", notificationID, err)
	}

	return notification, nil
}

func (r *PostgresNotificationRepository) ListForUser(
	ctx context.Context,
	userID string,
) ([]*domain.Notification, error) {
	rows, err := r.pool.Query(ctx, listNotificationsSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("listing notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]*domain.Notification, 0)

	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("listing notifications: %w", err)
		}

		notifications = append(notifications, notification)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing notifications: %w", err)
	}

	return notifications, nil
}

func (r *PostgresNotificationRepository) MarkAllRead(
	ctx context.Context,
	userID string,
	now time.Time,
) (int, error) {
	tag, err := r.pool.Exec(ctx, markAllReadSQL, userID, now)
	if err != nil {
		return 0, fmt.Errorf("marking notifications read: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

// scanNotification reads a row, putting the kind through the domain's parser so
// that a row holding something nobody has written a message for fails here
// rather than reaching a reader blank.
func scanNotification(row pgx.Row) (*domain.Notification, error) {
	var (
		notification domain.Notification
		kind         string
		readAt       *time.Time
	)

	err := row.Scan(
		&notification.ID, &notification.UserID, &kind, &notification.EventID,
		&notification.EventName, &notification.SeatID, &notification.CreatedAt, &readAt)
	if err != nil {
		return nil, err
	}

	if notification.Kind, err = domain.ParseNotificationKind(kind); err != nil {
		return nil, err
	}

	notification.ReadAt = instant(readAt)

	return &notification, nil
}

var (
	_ NotificationRepository = (*PostgresNotificationRepository)(nil)
	_ NotificationRepository = (*MemoryNotificationRepository)(nil)
)
