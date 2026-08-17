// Package service drives the reservation flow: it decides when a seat is held,
// confirmed or released, and leaves the rules themselves to the domain.
package service

import (
	"context"
	"crypto/rand"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// DefaultHoldTTL is how long a hold lives unless the caller says otherwise.
const DefaultHoldTTL = 5 * time.Minute

// ReservationService is the entry point for every seat operation.
type ReservationService struct {
	seats   repository.SeatRepository
	clock   Clock
	newID   func() string
	holdTTL time.Duration
}

// NewReservationService wires a service to its store, clock and id source. The
// id source is a parameter so that tests can hand out predictable hold ids.
func NewReservationService(
	seats repository.SeatRepository,
	clock Clock,
	newID func() string,
	holdTTL time.Duration,
) *ReservationService {
	return &ReservationService{
		seats:   seats,
		clock:   clock,
		newID:   newID,
		holdTTL: holdTTL,
	}
}

func NewRandomID() string {
	return rand.Text()
}

// HoldSeat claims a seat for a user and returns the resulting hold. The claim
// runs inside UpdateSeat so that checking whether the seat is free and taking it
// cannot be split apart by another caller.
func (s *ReservationService) HoldSeat(ctx context.Context, eventID, seatID, userID string) (*domain.Hold, error) {
	var (
		hold   *domain.Hold
		now    = s.clock.Now()
		holdID = s.newID()
	)

	err := s.seats.UpdateSeat(ctx, eventID, seatID, func(seat *domain.Seat) error {
		if err := seat.Hold(holdID, userID, s.holdTTL, now); err != nil {
			return err
		}

		hold = seat.CurrentHold(now)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return hold, nil
}

// ConfirmReservation turns a hold into a reservation and returns the confirmed
// seat. Only the user holding the seat may confirm it, and only before the hold
// runs out.
func (s *ReservationService) ConfirmReservation(ctx context.Context, holdID, userID string) (*domain.Seat, error) {
	var (
		confirmed *domain.Seat
		now       = s.clock.Now()
	)

	err := s.seats.UpdateSeatByHoldID(ctx, holdID, func(seat *domain.Seat) error {
		if err := seat.Confirm(userID, now); err != nil {
			return err
		}

		result := *seat
		confirmed = &result

		return nil
	})
	if err != nil {
		return nil, err
	}

	return confirmed, nil
}

// ReleaseSeat gives up a hold, making the seat available again straight away
// instead of waiting for the sweeper to notice.
func (s *ReservationService) ReleaseSeat(ctx context.Context, holdID, userID string) error {
	return s.seats.UpdateSeatByHoldID(ctx, holdID, func(seat *domain.Seat) error {
		return seat.Release(userID)
	})
}

// Seat returns a single seat, for the read side of the API.
func (s *ReservationService) Seat(ctx context.Context, eventID, seatID string) (*domain.Seat, error) {
	return s.seats.GetSeat(ctx, eventID, seatID)
}

// SeatMap returns every seat of an event in a stable order.
func (s *ReservationService) SeatMap(ctx context.Context, eventID string) ([]*domain.Seat, error) {
	return s.seats.ListSeats(ctx, eventID)
}
