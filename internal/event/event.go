package event

import "time"

// Kind says what happened. A small closed set, because every kind has to mean
// something to a subscriber, and a bag of arbitrary strings would not.
type Kind string

const (
	SeatChanged Kind = "seat_changed"

	TurnCame Kind = "turn_came"
)

type Event struct {
	Kind    Kind      `json:"kind"`
	EventID string    `json:"eventId"`
	SeatID  string    `json:"seatId,omitempty"`
	HoldID  string    `json:"holdId,omitempty"`
	At      time.Time `json:"at"`
	UserID  string    `json:"-"`
}

// IsForEveryone reports whether the notice goes to every watcher of its event.
func (e Event) IsForEveryone() bool {
	return e.UserID == ""
}
