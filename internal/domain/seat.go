package domain

import "time"

// Seat is a single seat of an event, and the only place where the rules about
// holding, confirming and releasing it live. The current hold is stored inline
// rather than as a separate object, so one Seat value carries the whole
// invariant and stage 2 will have exactly one thing to guard.
type Seat struct {
	ID      string
	EventID string
	Row     string
	Number  int

	Status SeatStatus

	// Only meaningful while Status is StatusHeld, and cleared on every
	// transition out of it. HoldCreatedAt is kept because once a hold is gone,
	// when it started cannot be reconstructed from anything else.
	HoldID        string
	HeldBy        string
	HoldCreatedAt time.Time
	HoldExpiresAt time.Time

	// ReservedBy is set once the seat has been confirmed.
	ReservedBy string
}

// NewSeat returns an available seat.
func NewSeat(eventID, id, row string, number int) *Seat {
	return &Seat{
		ID:      id,
		EventID: eventID,
		Row:     row,
		Number:  number,
		Status:  StatusAvailable,
	}
}

// IsHoldExpired reports whether the seat carries a hold that has run out by now.
// Only a held seat can have one, so a stale timestamp left on a seat in any
// other status is never read as expiry.
func (s *Seat) IsHoldExpired(now time.Time) bool {
	return s.Status == StatusHeld && reached(s.HoldExpiresAt, now)
}

// CanHold reports whether the seat may be held at now. A seat whose hold has
// expired is holdable again even though its status still says held: expiry is
// defined by the clock, not by whenever the sweeper gets around to it.
func (s *Seat) CanHold(now time.Time) bool {
	switch s.Status {
	case StatusAvailable:
		return true
	case StatusHeld:
		return s.IsHoldExpired(now)
	case StatusReserved:
		return false
	default:
		return false
	}
}

// Hold claims the seat for userID until now.Add(duration). A live hold blocks
// the seat even when userID owns that hold, so re-holding surfaces as
// ErrSeatNotAvailable instead of silently extending the hold.
func (s *Seat) Hold(holdID, userID string, duration time.Duration, now time.Time) error {
	if holdID == "" {
		return ErrEmptyHoldID
	}
	if userID == "" {
		return ErrEmptyUserID
	}
	if duration <= 0 {
		return ErrInvalidHoldDuration
	}
	if !s.CanHold(now) {
		return ErrSeatNotAvailable
	}

	s.Status = StatusHeld
	s.HoldID = holdID
	s.HeldBy = userID
	s.HoldCreatedAt = now
	s.HoldExpiresAt = now.Add(duration)
	s.ReservedBy = ""

	return nil
}

// Confirm turns the caller's live hold into a reservation. Ownership is checked
// before expiry, so a stranger gets ErrNotHoldOwner either way and the error
// never leaks the state of someone else's hold.
func (s *Seat) Confirm(userID string, now time.Time) error {
	if userID == "" {
		return ErrEmptyUserID
	}
	if s.Status != StatusHeld {
		return ErrSeatNotHeld
	}
	if s.HeldBy != userID {
		return ErrNotHoldOwner
	}
	if s.IsHoldExpired(now) {
		return ErrHoldExpired
	}

	s.Status = StatusReserved
	s.ReservedBy = userID
	s.clearHold()

	return nil
}

// Release drops the hold on behalf of the user that owns it. An expired hold can
// still be released by its owner, since the seat ends up free either way.
func (s *Seat) Release(userID string) error {
	if userID == "" {
		return ErrEmptyUserID
	}
	if s.Status != StatusHeld {
		return ErrSeatNotHeld
	}
	if s.HeldBy != userID {
		return ErrNotHoldOwner
	}

	s.Status = StatusAvailable
	s.clearHold()

	return nil
}

// ExpireHold drops the hold if it has run out by now and reports whether it
// changed anything. It takes no user because expiry is nobody's decision; the
// stage 2 sweeper is what calls it.
func (s *Seat) ExpireHold(now time.Time) bool {
	if !s.IsHoldExpired(now) {
		return false
	}

	s.Status = StatusAvailable
	s.clearHold()

	return true
}

// CurrentHold returns the live hold on the seat at now, or nil if there is none.
// The result is a copy, so mutating it does not affect the seat.
func (s *Seat) CurrentHold(now time.Time) *Hold {
	if s.Status != StatusHeld || s.IsHoldExpired(now) {
		return nil
	}

	return &Hold{
		ID:        s.HoldID,
		EventID:   s.EventID,
		SeatID:    s.ID,
		UserID:    s.HeldBy,
		CreatedAt: s.HoldCreatedAt,
		ExpiresAt: s.HoldExpiresAt,
	}
}

// clearHold wipes the hold fields. Every transition out of StatusHeld goes
// through here, so stale hold data cannot outlive the hold itself.
func (s *Seat) clearHold() {
	s.HoldID = ""
	s.HeldBy = ""
	s.HoldCreatedAt = time.Time{}
	s.HoldExpiresAt = time.Time{}
}
