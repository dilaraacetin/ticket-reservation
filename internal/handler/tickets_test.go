package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/service"
)

func ticketOf(eventID, name, seatID string, status domain.SeatStatus, holdExpiry time.Time) service.Ticket {
	seat := domain.NewSeat(eventID, seatID, string(seatID[0]), 1)
	seat.Status = status

	switch status {
	case domain.StatusHeld:
		seat.HoldID = "hold-" + seatID
		seat.HeldBy = testUser
		seat.HoldCreatedAt = testTime()
		seat.HoldExpiresAt = holdExpiry
	case domain.StatusReserved:
		seat.ReservedBy = testUser
	case domain.StatusAvailable:
	}

	return service.Ticket{
		Event: &domain.Event{ID: eventID, Name: name, Venue: "Volkswagen Arena", StartsAt: testTime().Add(72 * time.Hour)},
		Seat:  seat,
	}
}

// A page that is reloaded during a hold has lost the hold id and cannot confirm
// the seat it is still holding. This is where it gets it back.
func TestTickets_SeparatesHoldsFromReservations(t *testing.T) {
	svc := &fakeService{tickets: []service.Ticket{
		ticketOf("event-1", "Radiohead", "A1", domain.StatusHeld, testTime().Add(4*time.Minute)),
		ticketOf("event-1", "Radiohead", "B2", domain.StatusReserved, time.Time{}),
	}}

	rec := do(svc, http.MethodGet, "/tickets", withUser(testUser))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	body := decode[ticketsResponse](t, rec)

	if len(body.Holds) != 1 || body.Holds[0].SeatID != "A1" {
		t.Fatalf("holds = %v, want one A1", body.Holds)
	}
	if len(body.Reservations) != 1 || body.Reservations[0].SeatID != "B2" {
		t.Fatalf("reservations = %v, want one B2", body.Reservations)
	}

	hold := body.Holds[0]

	// Without these the page can show the seat but not act on it.
	if hold.HoldID == "" {
		t.Error("the hold carries no id, so the page still cannot confirm it")
	}
	if hold.ExpiresInSeconds <= 0 {
		t.Errorf("ExpiresInSeconds = %d, want the time left on the hold", hold.ExpiresInSeconds)
	}

	// Enough of the event to show the ticket without a second request.
	if hold.EventName != "Radiohead" || hold.Venue == "" {
		t.Errorf("the ticket does not carry its event: %+v", hold)
	}

	// A reservation is not a countdown.
	if body.Reservations[0].HoldID != "" || body.Reservations[0].ExpiresInSeconds != 0 {
		t.Errorf("a reservation carries hold fields: %+v", body.Reservations[0])
	}
}

// An expired hold is nobody's. The sweeper has simply not been round yet, and
// showing it would promise a seat that is already free for anyone to take.
func TestTickets_LeavesOutAnExpiredHold(t *testing.T) {
	svc := &fakeService{tickets: []service.Ticket{
		ticketOf("event-1", "Radiohead", "A1", domain.StatusHeld, testTime().Add(-time.Second)),
	}}

	body := decode[ticketsResponse](t, do(svc, http.MethodGet, "/tickets", withUser(testUser)))

	if len(body.Holds) != 0 {
		t.Errorf("holds = %v, want none; that hold has run out", body.Holds)
	}
}

func TestTickets_NeedsAToken(t *testing.T) {
	rec := do(&fakeService{}, http.MethodGet, "/tickets", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// Empty slices rather than null, so a client can loop over the answer without
// checking it first.
func TestTickets_EmptyIsNotNull(t *testing.T) {
	rec := do(&fakeService{}, http.MethodGet, "/tickets", withUser(testUser))

	if got := rec.Body.String(); !strings.Contains(got, `"holds":[]`) || !strings.Contains(got, `"reservations":[]`) {
		t.Errorf("body = %s, want empty arrays", got)
	}
}
