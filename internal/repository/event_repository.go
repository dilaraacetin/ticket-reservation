package repository

import (
	"context"

	"ticket-reservation/internal/domain"
)

// EventRepository stores events.
type EventRepository interface {
	GetEvent(ctx context.Context, eventID string) (*domain.Event, error)
	ListEvents(ctx context.Context) ([]*domain.Event, error)

	// CreateEvent stores a new event, or reports ErrEventExists. Separate from
	// the seeding helpers because stocking the service is now something a
	// request does, not only something a startup flag does.
	CreateEvent(ctx context.Context, event *domain.Event) error

	// UpdateEvent runs mutate against the stored event while it is locked, the
	// same shape UpdateSeat uses: reading, deciding and writing cannot be split
	// apart by another caller.
	UpdateEvent(ctx context.Context, eventID string, mutate func(*domain.Event) error) error

	// DeleteEvent removes an event outright, and refuses while any of its seats
	// is held or sold. Removing a mistake is one thing; removing something
	// people have tickets to is another, and withdrawing is what that is for.
	DeleteEvent(ctx context.Context, eventID string) error
}
