package domain

import "time"

// Hold is the addressable form of a seat's current hold; the authoritative
// state lives on the Seat itself. It carries EventID and SeatID so that a hold
// id coming from a URL can be resolved back to the seat that holds it.
type Hold struct {
	ID        string
	EventID   string
	SeatID    string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// IsExpired reports whether the hold has run out by now. See reached for the
// boundary rule.
func (h *Hold) IsExpired(now time.Time) bool {
	return reached(h.ExpiresAt, now)
}

// RemainingTime returns how long the hold still has at now, or zero if it has
// expired. It never returns a negative duration, so the result can be handed
// straight to a timer.
func (h *Hold) RemainingTime(now time.Time) time.Duration {
	if h.IsExpired(now) {
		return 0
	}

	return h.ExpiresAt.Sub(now)
}
