package handler

import (
	"time"

	"ticket-reservation/internal/domain"
)

type eventResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Venue      string    `json:"venue"`
	StartsAt   time.Time `json:"startsAt"`
	HasStarted bool      `json:"hasStarted"`
}

func newEventResponse(event *domain.Event, now time.Time) eventResponse {
	return eventResponse{
		ID:         event.ID,
		Name:       event.Name,
		Venue:      event.Venue,
		StartsAt:   event.StartsAt,
		HasStarted: event.HasStarted(now),
	}
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
	EventID string `json:"eventId"`
	SeatID  string `json:"seatId"`
	UserID  string `json:"userId"`
	Status  string `json:"status"`
}

func newReservationResponse(seat *domain.Seat) reservationResponse {
	return reservationResponse{
		EventID: seat.EventID,
		SeatID:  seat.ID,
		UserID:  seat.ReservedBy,
		Status:  seat.Status.String(),
	}
}

type healthResponse struct {
	Status string `json:"status"`
}
