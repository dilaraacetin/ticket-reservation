package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/domain"
)

// DeliveryLease is how long a claimed delivery is left alone before another
// worker may pick it up.
//
// It exists because a worker can die between claiming a delivery and recording
// what happened to it. Without the lease that delivery would sit claimed for
// ever; with it, the work comes back round. The cost is that a message can go
// out twice, which for a notification is the better of the two mistakes.
const DeliveryLease = time.Minute

type PostgresDeliveryRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresDeliveryRepository(pool *pgxpool.Pool) *PostgresDeliveryRepository {
	return &PostgresDeliveryRepository{pool: pool}
}

const (
	enqueueDeliverySQL = `insert into notification_deliveries
		 (id, notification_id, user_id, channel, attempts, last_error, created_at, due_at)
		 values ($1, $2, $3, $4, 0, '', $5, $6)
		 on conflict (notification_id, channel) do nothing`

	// One statement. The rows are claimed and their due time pushed out in the
	// same breath, so two workers arriving together cannot both take the same
	// delivery and send the same message twice.
	//
	// skip locked is what lets the second worker get on with the next one
	// instead of waiting behind the first.
	takeDueDeliveriesSQL = `with due as (
		     select id
		       from notification_deliveries
		      where sent_at is null
		        and gave_up_at is null
		        and due_at <= $1
		      order by due_at, id
		      limit $3
		        for update skip locked
		 )
		 update notification_deliveries d
		    set due_at = $2
		   from due
		  where d.id = due.id
	   returning ` + `d.id, d.notification_id, d.user_id, d.channel,
		 d.attempts, d.last_error, d.created_at, d.due_at, d.sent_at, d.gave_up_at`

	saveDeliverySQL = `update notification_deliveries
		   set attempts = $2, last_error = $3, due_at = $4, sent_at = $5, gave_up_at = $6
		 where id = $1`

	pendingDeliveriesSQL = `select count(*) from notification_deliveries
		 where sent_at is null and gave_up_at is null`
)

func (r *PostgresDeliveryRepository) Enqueue(ctx context.Context, deliveries ...*domain.Delivery) error {
	if len(deliveries) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, delivery := range deliveries {
		batch.Queue(enqueueDeliverySQL,
			delivery.ID, delivery.NotificationID, delivery.UserID, delivery.Channel.String(),
			delivery.CreatedAt, delivery.DueAt)
	}

	if err := r.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("queueing deliveries: %w", err)
	}

	return nil
}

func (r *PostgresDeliveryRepository) TakeDue(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]*domain.Delivery, error) {
	rows, err := r.pool.Query(ctx, takeDueDeliveriesSQL, now, now.Add(DeliveryLease), limit)
	if err != nil {
		return nil, fmt.Errorf("claiming deliveries: %w", err)
	}
	defer rows.Close()

	claimed := make([]*domain.Delivery, 0)

	for rows.Next() {
		delivery, err := scanDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("claiming deliveries: %w", err)
		}

		claimed = append(claimed, delivery)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claiming deliveries: %w", err)
	}

	return claimed, nil
}

func (r *PostgresDeliveryRepository) Save(ctx context.Context, delivery *domain.Delivery) error {
	tag, err := r.pool.Exec(ctx, saveDeliverySQL,
		delivery.ID, delivery.Attempts, delivery.LastError, delivery.DueAt,
		nullInstant(delivery.SentAt), nullInstant(delivery.GaveUpAt))
	if err != nil {
		return fmt.Errorf("saving a delivery: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrDeliveryNotFound
	}

	return nil
}

func (r *PostgresDeliveryRepository) Pending(ctx context.Context) (int, error) {
	var pending int
	if err := r.pool.QueryRow(ctx, pendingDeliveriesSQL).Scan(&pending); err != nil {
		return 0, fmt.Errorf("counting pending deliveries: %w", err)
	}

	return pending, nil
}

func scanDelivery(row pgx.Row) (*domain.Delivery, error) {
	var (
		delivery domain.Delivery
		channel  string
		sentAt   *time.Time
		gaveUpAt *time.Time
	)

	err := row.Scan(
		&delivery.ID, &delivery.NotificationID, &delivery.UserID, &channel,
		&delivery.Attempts, &delivery.LastError, &delivery.CreatedAt, &delivery.DueAt,
		&sentAt, &gaveUpAt)
	if err != nil {
		return nil, err
	}

	// Through the domain's parser, so a row holding a channel nobody has written
	// a sender for fails here rather than being claimed and quietly dropped.
	if delivery.Channel, err = domain.ParseDeliveryChannel(channel); err != nil {
		return nil, err
	}

	delivery.SentAt = instant(sentAt)
	delivery.GaveUpAt = instant(gaveUpAt)

	return &delivery, nil
}

var (
	_ DeliveryRepository = (*PostgresDeliveryRepository)(nil)
	_ DeliveryRepository = (*MemoryDeliveryRepository)(nil)
)
