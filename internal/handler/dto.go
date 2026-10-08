package handler

import (
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/service"
)

type eventResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Venue      string    `json:"venue"`
	StartsAt   time.Time `json:"startsAt"`
	HasStarted bool      `json:"hasStarted"`

	City        string `json:"city"`
	Category    string `json:"category"`
	ImageURL    string `json:"imageUrl"`
	Description string `json:"description"`
	Rules       string `json:"rules"`

	// A withdrawn event stays in the catalogue rather than disappearing, so that
	// somebody holding a ticket to it can see what happened.
	Cancelled   bool      `json:"cancelled"`
	CancelledAt time.Time `json:"cancelledAt,omitzero"`
}

func newEventResponse(event *domain.Event, now time.Time) eventResponse {
	return eventResponse{
		ID:          event.ID,
		Name:        event.Name,
		Venue:       event.Venue,
		StartsAt:    event.StartsAt,
		HasStarted:  event.HasStarted(now),
		City:        event.Details.City,
		Category:    event.Details.Category.String(),
		ImageURL:    event.Details.ImageURL,
		Description: event.Details.Description,
		Rules:       event.Details.Rules,
		Cancelled:   event.IsCancelled(),
		CancelledAt: event.CancelledAt,
	}
}

// eventDetailResponse is one event with the two numbers the detail page leads
// on. The list does not carry them: counting every event's seats to draw a
// catalogue is a lot of work for a line of text.
type eventDetailResponse struct {
	eventResponse

	Available int `json:"available"`
	Total     int `json:"total"`
}

type seatResponse struct {
	ID     string `json:"id"`
	Row    string `json:"row"`
	Number int    `json:"number"`
	Status string `json:"status"`
}

func newSeatResponse(seat *domain.Seat, now time.Time) seatResponse {
	status := seat.Status
	if seat.IsHoldExpired(now) {
		status = domain.StatusAvailable
	}

	return seatResponse{
		ID:     seat.ID,
		Row:    seat.Row,
		Number: seat.Number,
		Status: status.String(),
	}
}

type seatMapResponse struct {
	EventID string         `json:"eventId"`
	Seats   []seatResponse `json:"seats"`
}

type holdResponse struct {
	HoldID           string    `json:"holdId"`
	EventID          string    `json:"eventId"`
	SeatID           string    `json:"seatId"`
	UserID           string    `json:"userId"`
	ExpiresAt        time.Time `json:"expiresAt"`
	ExpiresInSeconds int       `json:"expiresInSeconds"`
}

func newHoldResponse(hold *domain.Hold, now time.Time) holdResponse {
	return holdResponse{
		HoldID:           hold.ID,
		EventID:          hold.EventID,
		SeatID:           hold.SeatID,
		UserID:           hold.UserID,
		ExpiresAt:        hold.ExpiresAt,
		ExpiresInSeconds: int(hold.RemainingTime(now).Seconds()),
	}
}

type reservationResponse struct {
	EventID    string `json:"eventId"`
	SeatID     string `json:"seatId"`
	UserID     string `json:"userId"`
	Status     string `json:"status"`
	TicketCode string `json:"ticketCode"`
}

func newReservationResponse(seat *domain.Seat) reservationResponse {
	return reservationResponse{
		EventID:    seat.EventID,
		SeatID:     seat.ID,
		UserID:     seat.ReservedBy,
		Status:     seat.Status.String(),
		TicketCode: seat.TicketCode,
	}
}

type healthResponse struct {
	Status string `json:"status"`
}

// ticketResponse is a seat the caller has, with enough of its event to show it
// without a second request.
type ticketResponse struct {
	EventID   string    `json:"eventId"`
	EventName string    `json:"eventName"`
	Venue     string    `json:"venue"`
	StartsAt  time.Time `json:"startsAt"`
	SeatID    string    `json:"seatId"`
	Row       string    `json:"row"`
	Number    int       `json:"number"`
	Status    string    `json:"status"`

	// TicketCode is present once the seat is confirmed. It is what is shown and
	// scanned at the door.
	TicketCode string `json:"ticketCode,omitempty"`

	// Only set while the seat is held: what the client needs to confirm it, and
	// how long it has to decide.
	HoldID           string    `json:"holdId,omitempty"`
	ExpiresAt        time.Time `json:"expiresAt,omitzero"`
	ExpiresInSeconds int       `json:"expiresInSeconds,omitempty"`
}

func newTicketResponse(ticket service.Ticket, now time.Time) ticketResponse {
	body := ticketResponse{
		EventID:    ticket.Event.ID,
		EventName:  ticket.Event.Name,
		Venue:      ticket.Event.Venue,
		StartsAt:   ticket.Event.StartsAt,
		SeatID:     ticket.Seat.ID,
		Row:        ticket.Seat.Row,
		Number:     ticket.Seat.Number,
		Status:     ticket.Seat.Status.String(),
		TicketCode: ticket.Seat.TicketCode,
	}

	if hold := ticket.Seat.CurrentHold(now); hold != nil {
		body.HoldID = hold.ID
		body.ExpiresAt = hold.ExpiresAt
		body.ExpiresInSeconds = int(hold.RemainingTime(now).Seconds())
	}

	return body
}

// ticketsResponse keeps the two apart, because they are different things to a
// reader: one is a countdown, the other is a ticket.
type ticketsResponse struct {
	Holds        []ticketResponse `json:"holds"`
	Reservations []ticketResponse `json:"reservations"`
}
