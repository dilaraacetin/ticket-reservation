package repository

import (
	"context"

	"ticket-reservation/internal/domain"
)

// EventRepository stores events.
type EventRepository interface {
	GetEvent(ctx context.Context, eventID string) (*domain.Event, error)
	ListEvents(ctx context.Context) ([]*domain.Event, error)
}
