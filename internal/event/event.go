package event

import "time"

// Kind says what happened. A small closed set, because every kind has to mean
// something to a subscriber, and a bag of arbitrary strings would not.
type Kind string

const (
	SeatChanged Kind = "seat_changed"

	TurnCame Kind = "turn_came"

	// EventCancelled goes to everyone watching the event. Public, because it is
	// not news about one person; a durable notice is stored separately for the
	// people who have tickets and are not looking.
	EventCancelled Kind = "event_cancelled"
)

type Event struct {
	Kind    Kind   `json:"kind"`
	EventID string `json:"eventId"`
	SeatID  string `json:"seatId,omitempty"`
	HoldID  string `json:"holdId,omitempty"`

	// Only set on turn_came. A notice that hands somebody a hold has to say when
	// it runs out, or the client has no way to show them how long they have.
	ExpiresAt time.Time `json:"expiresAt,omitzero"`

	At     time.Time `json:"at"`
	UserID string    `json:"-"`
}

// IsForEveryone reports whether the notice goes to every watcher of its event.
func (e Event) IsForEveryone() bool {
	return e.UserID == ""
}
