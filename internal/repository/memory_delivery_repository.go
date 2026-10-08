package repository

import (
	"context"
	"slices"
	"sync"
	"time"

	"ticket-reservation/internal/domain"
)

// MemoryDeliveryRepository keeps the outbox in a map.
type MemoryDeliveryRepository struct {
	mu         sync.Mutex
	deliveries map[string]*domain.Delivery
}

func NewMemoryDeliveryRepository() *MemoryDeliveryRepository {
	return &MemoryDeliveryRepository{deliveries: make(map[string]*domain.Delivery)}
}

// key is what the unique constraint is in Postgres: one delivery per notice per
// channel.
func deliveryKey(notificationID string, channel domain.DeliveryChannel) string {
	return notificationID + "/" + channel.String()
}

func (r *MemoryDeliveryRepository) Enqueue(_ context.Context, deliveries ...*domain.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, delivery := range deliveries {
		key := deliveryKey(delivery.NotificationID, delivery.Channel)
		if _, queued := r.deliveries[key]; queued {
			continue
		}

		stored := *delivery
		r.deliveries[key] = &stored
	}

	return nil
}

// TakeDue claims due deliveries under the lock and pushes their due time out, so
// a second worker arriving immediately does not get the same ones.
func (r *MemoryDeliveryRepository) TakeDue(
	_ context.Context,
	now time.Time,
	limit int,
) ([]*domain.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]string, 0, len(r.deliveries))
	for key, delivery := range r.deliveries {
		if delivery.IsDone() || now.Before(delivery.DueAt) {
			continue
		}

		keys = append(keys, key)
	}

	// Oldest due first, with the key breaking ties so the order is definite.
	slices.SortFunc(keys, func(a, b string) int {
		if due := r.deliveries[a].DueAt.Compare(r.deliveries[b].DueAt); due != 0 {
			return due
		}

		return cmpString(a, b)
	})

	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}

	claimed := make([]*domain.Delivery, 0, len(keys))

	for _, key := range keys {
		r.deliveries[key].DueAt = now.Add(DeliveryLease)

		taken := *r.deliveries[key]
		claimed = append(claimed, &taken)
	}

	return claimed, nil
}

func (r *MemoryDeliveryRepository) Save(_ context.Context, delivery *domain.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := deliveryKey(delivery.NotificationID, delivery.Channel)
	if _, queued := r.deliveries[key]; !queued {
		return ErrDeliveryNotFound
	}

	stored := *delivery
	r.deliveries[key] = &stored

	return nil
}

func (r *MemoryDeliveryRepository) Pending(_ context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pending := 0

	for _, delivery := range r.deliveries {
		if !delivery.IsDone() {
			pending++
		}
	}

	return pending, nil
}
