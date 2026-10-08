package repository

import (
	"context"
	"slices"
	"sync"

	"ticket-reservation/internal/domain"
)

// MemoryPushRepository keeps subscriptions keyed by endpoint.
type MemoryPushRepository struct {
	mu            sync.RWMutex
	subscriptions map[string]*domain.PushSubscription
}

func NewMemoryPushRepository() *MemoryPushRepository {
	return &MemoryPushRepository{subscriptions: make(map[string]*domain.PushSubscription)}
}

func (r *MemoryPushRepository) Subscribe(_ context.Context, subscription *domain.PushSubscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored := *subscription
	r.subscriptions[stored.Endpoint] = &stored

	return nil
}

func (r *MemoryPushRepository) ListForUser(
	_ context.Context,
	userID string,
) ([]*domain.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	found := make([]*domain.PushSubscription, 0)

	for _, subscription := range r.subscriptions {
		if subscription.UserID != userID {
			continue
		}

		copied := *subscription
		found = append(found, &copied)
	}

	slices.SortFunc(found, func(a, b *domain.PushSubscription) int {
		return cmpString(a.Endpoint, b.Endpoint)
	})

	return found, nil
}

func (r *MemoryPushRepository) Unsubscribe(_ context.Context, endpoint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.subscriptions, endpoint)

	return nil
}
