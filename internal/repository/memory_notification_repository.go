package repository

import (
	"context"
	"slices"
	"sync"
	"time"

	"ticket-reservation/internal/domain"
)

// MemoryNotificationRepository keeps notices in a slice per person.
type MemoryNotificationRepository struct {
	mu     sync.RWMutex
	byUser map[string][]*domain.Notification
}

func NewMemoryNotificationRepository() *MemoryNotificationRepository {
	return &MemoryNotificationRepository{byUser: make(map[string][]*domain.Notification)}
}

func (r *MemoryNotificationRepository) Notify(_ context.Context, notifications ...*domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, notification := range notifications {
		stored := *notification
		r.byUser[stored.UserID] = append(r.byUser[stored.UserID], &stored)
	}

	return nil
}

// Get returns one notice, wherever it is.
func (r *MemoryNotificationRepository) Get(
	_ context.Context,
	notificationID string,
) (*domain.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, notices := range r.byUser {
		for _, notice := range notices {
			if notice.ID == notificationID {
				found := *notice

				return &found, nil
			}
		}
	}

	return nil, ErrNotificationNotFound
}

func (r *MemoryNotificationRepository) ListForUser(
	_ context.Context,
	userID string,
) ([]*domain.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stored := r.byUser[userID]
	notifications := make([]*domain.Notification, 0, len(stored))

	for _, notification := range stored {
		copied := *notification
		notifications = append(notifications, &copied)
	}

	// Newest first, with the id breaking ties so two notices written in the same
	// instant still come back in a definite order.
	slices.SortFunc(notifications, func(a, b *domain.Notification) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return cmpString(a.ID, b.ID)
		}

		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}

		return 1
	})

	return notifications, nil
}

func (r *MemoryNotificationRepository) MarkAllRead(
	_ context.Context,
	userID string,
	now time.Time,
) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	marked := 0

	for _, notification := range r.byUser[userID] {
		if notification.IsRead() {
			continue
		}

		notification.ReadAt = now
		marked++
	}

	return marked, nil
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
