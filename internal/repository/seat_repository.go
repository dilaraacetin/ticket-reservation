// Package repository defines how seats are stored and provides the
// implementations.
package repository

import (
	"context"
	"time"

	"ticket-reservation/internal/domain"
)

// SeatRepository stores seats and hands them back.
type SeatRepository interface {
	GetSeat(ctx context.Context, eventID, seatID string) (*domain.Seat, error)

	ListSeats(ctx context.Context, eventID string) ([]*domain.Seat, error)

	UpdateSeat(ctx context.Context, eventID, seatID string, mutate func(*domain.Seat) error) error

	UpdateSeatByHoldID(ctx context.Context, holdID string, mutate func(*domain.Seat) error) error

	// ExpireHolds frees every hold that has run out and names the seats it
	// freed, so that whoever is waiting for one can be offered it.
	// ListSeatsForUser returns every seat the user is holding or has reserved,
	// across all events. One method rather than two, because the status already
	// tells the two apart and the caller usually wants both at once.
	ListSeatsForUser(ctx context.Context, userID string) ([]*domain.Seat, error)

	ExpireHolds(ctx context.Context, now time.Time) ([]domain.SeatRef, error)

	// CreateSeats adds seats to an event and reports how many it actually added.
	// Seats that already exist are left alone, so adding a row twice does not
	// disturb one that is already sold, and the count says how many were new.
	CreateSeats(ctx context.Context, seats ...*domain.Seat) (int, error)
}
