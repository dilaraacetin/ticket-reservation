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

	ExpireHolds(ctx context.Context, now time.Time) (int, error)
}
