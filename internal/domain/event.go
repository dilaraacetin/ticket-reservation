package domain

import "time"

// Event is something people buy seats for: a concert, a screening, a departure.
// Seats are stored per event by the repository rather than embedded here,
// because a hall can hold thousands of them and almost every operation touches
// exactly one.
type Event struct {
	ID       string
	Name     string
	Venue    string
	StartsAt time.Time
}

// HasStarted reports whether the event has already begun at now. Whether a
// started event still sells is a policy call, so it is left to the service
// layer.
func (e *Event) HasStarted(now time.Time) bool {
	return reached(e.StartsAt, now)
}
