package repository

import (
	"context"

	"ticket-reservation/internal/domain"
)

// PushRepository stores which browsers have agreed to be pushed to.
type PushRepository interface {
	// Subscribe stores a subscription. The same endpoint arriving again replaces
	// what is there: a browser that re-subscribes has new keys, and the old ones
	// no longer work.
	Subscribe(ctx context.Context, subscription *domain.PushSubscription) error

	// ListForUser returns everything a person has subscribed.
	ListForUser(ctx context.Context, userID string) ([]*domain.PushSubscription, error)

	// Unsubscribe forgets one endpoint. Also how a dead subscription is cleared:
	// a push service that says an endpoint is gone is telling the truth, and
	// keeping it would mean failing against it for ever.
	Unsubscribe(ctx context.Context, endpoint string) error
}
