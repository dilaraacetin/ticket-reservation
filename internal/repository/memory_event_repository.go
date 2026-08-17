package repository

import (
	"context"
	"slices"
	"strings"
	"sync"

	"ticket-reservation/internal/domain"
)

type MemoryEventRepository struct {
	mu     sync.RWMutex
	events map[string]*domain.Event
}

func NewMemoryEventRepository(events ...*domain.Event) *MemoryEventRepository {
	r := &MemoryEventRepository{events: make(map[string]*domain.Event, len(events))}
	for _, event := range events {
		stored := *event
		r.events[event.ID] = &stored
	}

	return r
}

func (r *MemoryEventRepository) GetEvent(_ context.Context, eventID string) (*domain.Event, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stored, ok := r.events[eventID]
	if !ok {
		return nil, ErrEventNotFound
	}

	event := *stored

	return &event, nil
}

func (r *MemoryEventRepository) ListEvents(_ context.Context) ([]*domain.Event, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	events := make([]*domain.Event, 0, len(r.events))
	for _, stored := range r.events {
		event := *stored
		events = append(events, &event)
	}

	slices.SortFunc(events, func(a, b *domain.Event) int {
		if when := a.StartsAt.Compare(b.StartsAt); when != 0 {
			return when
		}

		return strings.Compare(a.ID, b.ID)
	})

	return events, nil
}
