// Package service drives the reservation flow: it decides when a seat is held,
// confirmed or released, and leaves the rules themselves to the domain.
package service

import (
	"context"
	"crypto/rand"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/event"
	"ticket-reservation/internal/repository"
)

// DefaultHoldTTL is how long a hold lives unless the caller says otherwise.
const DefaultHoldTTL = 5 * time.Minute

// ReservationService is the entry point for every seat operation.
type ReservationService struct {
	seats     repository.SeatRepository
	events    repository.EventRepository
	clock     Clock
	newID     func() string
	holdTTL   time.Duration
	publisher event.Publisher
	offerer   SeatOfferer
	gate      SeatGate
}

// Config carries the service's dependencies. A struct rather than a list of
// parameters, because two of them are a func and a Duration and passing those
// in the wrong order would still compile.
type Config struct {
	Seats     repository.SeatRepository
	Events    repository.EventRepository
	Clock     Clock
	NewID     func() string
	HoldTTL   time.Duration
	Publisher event.Publisher

	// Gate decides who may take a seat. Defaults to letting everyone, which is
	// what the tests about seats use; a deployment passes the real rule.
	Gate SeatGate
}

// NewReservationService wires a service to its stores, clock and id source. The
// id source is injected so that tests can hand out predictable hold ids.
func NewReservationService(cfg Config) *ReservationService {
	if cfg.Publisher == nil {
		cfg.Publisher = event.Discard{}
	}

	if cfg.Gate == nil {
		cfg.Gate = AllowEveryone{}
	}

	return &ReservationService{
		seats:     cfg.Seats,
		events:    cfg.Events,
		clock:     cfg.Clock,
		newID:     cfg.NewID,
		holdTTL:   cfg.HoldTTL,
		publisher: cfg.Publisher,
		offerer:   discardOfferer{},
		gate:      cfg.Gate,
	}
}

// WithOfferer wires in what happens to a seat this service frees. Optional and
// set afterwards, because the handoff needs this service to make its holds and
// the two cannot both be built first.
func (s *ReservationService) WithOfferer(offerer SeatOfferer) *ReservationService {
	s.offerer = offerer

	return s
}

// announceSeatChange tells watchers a seat is no longer what they last saw.
func (s *ReservationService) announceSeatChange(eventID, seatID string) {
	s.publisher.Publish(event.Event{
		Kind:    event.SeatChanged,
		EventID: eventID,
		SeatID:  seatID,
		At:      s.clock.Now(),
	})
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

	if err := s.gate.MayTakeSeat(ctx, userID); err != nil {
		return nil, err
	}

	// One extra read on the busiest path, and worth it: without it a withdrawn
	// event keeps selling. Confirming is deliberately not guarded the same way —
	// somebody already holding a seat was part way through a purchase when the
	// event was pulled, and letting them finish is the kinder answer.
	event, err := s.events.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	if event.IsCancelled() {
		return nil, domain.ErrEventCancelled
	}

	err = s.seats.UpdateSeat(ctx, eventID, seatID, func(seat *domain.Seat) error {
		if err := seat.Hold(holdID, userID, s.holdTTL, now); err != nil {
			return err
		}

		hold = seat.CurrentHold(now)

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.announceSeatChange(eventID, seatID)

	return hold, nil
}

// ConfirmReservation turns a hold into a reservation and returns the confirmed
// seat. Only the user holding the seat may confirm it, and only before the hold
// runs out.
func (s *ReservationService) ConfirmReservation(ctx context.Context, holdID, userID string) (*domain.Seat, error) {
	// Checked here as well as on holding, because a hold made before the rule
	// applied would otherwise still turn into a sale.
	if err := s.gate.MayTakeSeat(ctx, userID); err != nil {
		return nil, err
	}

	var (
		confirmed *domain.Seat
		now       = s.clock.Now()
		// Generated outside the closure, so a retry of the update does not mint a
		// second code for the same ticket.
		ticketCode = domain.FormatTicketCode(s.newID()[:8])
	)

	err := s.seats.UpdateSeatByHoldID(ctx, holdID, func(seat *domain.Seat) error {
		if err := seat.Confirm(userID, ticketCode, now); err != nil {
			return err
		}

		result := *seat
		confirmed = &result

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.announceSeatChange(confirmed.EventID, confirmed.ID)

	return confirmed, nil
}

// ReleaseSeat gives up a hold, making the seat available again straight away
// instead of waiting for the sweeper to notice.
func (s *ReservationService) ReleaseSeat(ctx context.Context, holdID, userID string) error {
	var released *domain.Seat

	err := s.seats.UpdateSeatByHoldID(ctx, holdID, func(seat *domain.Seat) error {
		if err := seat.Release(userID); err != nil {
			return err
		}

		result := *seat
		released = &result

		return nil
	})
	if err != nil {
		return err
	}

	s.announceSeatChange(released.EventID, released.ID)
	s.offerer.Offer(released.Ref())

	return nil
}

// Ticket is a seat the caller has, with enough of its event to show it. Seats
// are stored per event, so the two have to be put back together somewhere, and
// the service is the layer that may know about both.
type Ticket struct {
	Event *domain.Event
	Seat  *domain.Seat
}

// MyTickets returns every seat the caller is holding or has reserved.
//
// Without this a confirmed seat is invisible: the seat map deliberately does
// not say who holds or owns a seat, so a caller who closes the page has nothing
// left that points back to it.
func (s *ReservationService) MyTickets(ctx context.Context, userID string) ([]Ticket, error) {
	if userID == "" {
		return nil, domain.ErrEmptyUserID
	}

	seats, err := s.seats.ListSeatsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// One lookup per event rather than per seat, because somebody with a whole
	// row has one event and ten seats.
	events := make(map[string]*domain.Event, len(seats))
	tickets := make([]Ticket, 0, len(seats))

	for _, seat := range seats {
		event, seen := events[seat.EventID]
		if !seen {
			if event, err = s.events.GetEvent(ctx, seat.EventID); err != nil {
				return nil, err
			}

			events[seat.EventID] = event
		}

		tickets = append(tickets, Ticket{Event: event, Seat: seat})
	}

	return tickets, nil
}

// Seat returns a single seat, for the read side of the API.
func (s *ReservationService) Seat(ctx context.Context, eventID, seatID string) (*domain.Seat, error) {
	return s.seats.GetSeat(ctx, eventID, seatID)
}

// Events returns every event, for the listing endpoint.
func (s *ReservationService) Events(ctx context.Context) ([]*domain.Event, error) {
	return s.events.ListEvents(ctx)
}

// Event returns a single event.
func (s *ReservationService) Event(ctx context.Context, eventID string) (*domain.Event, error) {
	return s.events.GetEvent(ctx, eventID)
}

// SeatMap returns every seat of an event in a stable order. The event is looked
// up first so that an unknown id is reported as missing instead of coming back
// as an empty seat map.
func (s *ReservationService) SeatMap(ctx context.Context, eventID string) ([]*domain.Seat, error) {
	if _, err := s.events.GetEvent(ctx, eventID); err != nil {
		return nil, err
	}

	return s.seats.ListSeats(ctx, eventID)
}
