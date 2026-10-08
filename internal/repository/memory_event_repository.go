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

	// seats is consulted before an event is deleted. Optional, because most uses
	// of this store have no seats to speak of; without it a delete is refused
	// only on the event's own account.
	seats *MemorySeatRepository
}

// WithSeats lets the store see whether an event still has seats that are held
// or sold, which is what a delete has to refuse on.
func (r *MemoryEventRepository) WithSeats(seats *MemorySeatRepository) *MemoryEventRepository {
	r.seats = seats

	return r
}

// storable is the copy this store keeps. The defaults are filled in here for the
// same reason the Postgres store fills them in on the way to a not-null column:
// the two have to answer a read the same way, whatever they were handed.
func storable(event *domain.Event) *domain.Event {
	stored := *event
	stored.Details = stored.Details.WithDefaults()

	return &stored
}

func NewMemoryEventRepository(events ...*domain.Event) *MemoryEventRepository {
	r := &MemoryEventRepository{events: make(map[string]*domain.Event, len(events))}
	for _, event := range events {
		r.events[event.ID] = storable(event)
	}

	return r
}

// CreateEvent stores a new event. The check and the write are both inside the
// lock, which is what the primary key does for the Postgres store.
func (r *MemoryEventRepository) CreateEvent(_ context.Context, event *domain.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.events[event.ID]; exists {
		return ErrEventExists
	}

	r.events[event.ID] = storable(event)

	return nil
}

// UpdateEvent runs mutate against the stored event under the write lock, so no
// other caller can read or write it in between.
func (r *MemoryEventRepository) UpdateEvent(
	_ context.Context,
	eventID string,
	mutate func(*domain.Event) error,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored, ok := r.events[eventID]
	if !ok {
		return ErrEventNotFound
	}

	event := *stored
	if err := mutate(&event); err != nil {
		return err
	}

	r.events[eventID] = storable(&event)

	return nil
}

// DeleteEvent removes an event unless somebody holds or owns one of its seats.
func (r *MemoryEventRepository) DeleteEvent(ctx context.Context, eventID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.events[eventID]; !ok {
		return ErrEventNotFound
	}

	if r.seats != nil {
		seats, err := r.seats.ListSeats(ctx, eventID)
		if err != nil {
			return err
		}

		for _, seat := range seats {
			if seat.Status != domain.StatusAvailable {
				return ErrEventInUse
			}
		}
	}

	delete(r.events, eventID)

	return nil
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
