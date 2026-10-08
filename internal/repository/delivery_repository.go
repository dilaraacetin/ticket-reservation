package repository

import (
	"context"
	"time"

	"ticket-reservation/internal/domain"
)

// DeliveryRepository is the outbox: what the service still owes people outside
// the application, and how far each attempt has got.
type DeliveryRepository interface {
	// Enqueue adds deliveries. A delivery that is already queued for the same
	// notice and channel is left alone, so an enqueue that is retried does not
	// send the same thing twice.
	Enqueue(ctx context.Context, deliveries ...*domain.Delivery) error

	// TakeDue claims up to limit deliveries that are due, and must hand each one
	// to exactly one caller: two workers running at once have to get two
	// different deliveries, or the same message goes out twice.
	TakeDue(ctx context.Context, now time.Time, limit int) ([]*domain.Delivery, error)

	// Save writes back an attempt's outcome.
	Save(ctx context.Context, delivery *domain.Delivery) error

	// Pending reports how many deliveries are still owed, for the metrics.
	Pending(ctx context.Context) (int, error)
}
