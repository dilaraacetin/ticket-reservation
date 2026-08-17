package domain

import "time"

// Reservation is a confirmed purchase: the seat belongs to the user and no
// expiry will take it away. It has no methods because it has no lifecycle.
type Reservation struct {
	ID        string
	EventID   string
	SeatID    string
	UserID    string
	CreatedAt time.Time
}
