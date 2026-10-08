package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/domain"
)

type PostgresPushRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresPushRepository(pool *pgxpool.Pool) *PostgresPushRepository {
	return &PostgresPushRepository{pool: pool}
}

const pushColumns = `id, user_id, endpoint, p256dh, auth, created_at`

const (
	// On conflict the keys are replaced rather than ignored: a browser that
	// re-subscribes keeps its endpoint but gets new keys, and the old ones would
	// no longer decrypt anything.
	subscribePushSQL = `insert into push_subscriptions (` + pushColumns + `)
		 values ($1, $2, $3, $4, $5, $6)
		 on conflict (endpoint) do update
		    set user_id = excluded.user_id,
		        p256dh  = excluded.p256dh,
		        auth    = excluded.auth`

	listPushForUserSQL = `select ` + pushColumns + `
		  from push_subscriptions
		 where user_id = $1
		 order by endpoint`

	unsubscribePushSQL = `delete from push_subscriptions where endpoint = $1`
)

func (r *PostgresPushRepository) Subscribe(
	ctx context.Context,
	subscription *domain.PushSubscription,
) error {
	_, err := r.pool.Exec(ctx, subscribePushSQL,
		subscription.ID, subscription.UserID, subscription.Endpoint,
		subscription.P256dh, subscription.Auth, subscription.CreatedAt)
	if err != nil {
		return fmt.Errorf("storing a push subscription: %w", err)
	}

	return nil
}

func (r *PostgresPushRepository) ListForUser(
	ctx context.Context,
	userID string,
) ([]*domain.PushSubscription, error) {
	rows, err := r.pool.Query(ctx, listPushForUserSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("listing push subscriptions: %w", err)
	}
	defer rows.Close()

	subscriptions := make([]*domain.PushSubscription, 0)

	for rows.Next() {
		var subscription domain.PushSubscription

		err := rows.Scan(&subscription.ID, &subscription.UserID, &subscription.Endpoint,
			&subscription.P256dh, &subscription.Auth, &subscription.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("listing push subscriptions: %w", err)
		}

		subscriptions = append(subscriptions, &subscription)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing push subscriptions: %w", err)
	}

	return subscriptions, nil
}

func (r *PostgresPushRepository) Unsubscribe(ctx context.Context, endpoint string) error {
	if _, err := r.pool.Exec(ctx, unsubscribePushSQL, endpoint); err != nil {
		return fmt.Errorf("forgetting a push subscription: %w", err)
	}

	return nil
}

var (
	_ PushRepository = (*PostgresPushRepository)(nil)
	_ PushRepository = (*MemoryPushRepository)(nil)
)
