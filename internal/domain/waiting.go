package domain

import "time"

// WaitingEntry is one user's place in an event's queue. The queue is per event
// rather than per seat, because someone waiting wants a ticket rather than one
// particular chair.
type WaitingEntry struct {
	ID       string
	EventID  string
	UserID   string
	JoinedAt time.Time
}

// NewWaitingEntry returns an entry ready to be stored. JoinedAt is what orders
// the queue, so it is taken from the caller's clock rather than the database's.
func NewWaitingEntry(id, eventID, userID string, now time.Time) (*WaitingEntry, error) {
	if id == "" {
		return nil, ErrEmptyEntryID
	}
	if eventID == "" {
		return nil, ErrEmptyEventID
	}
	if userID == "" {
		return nil, ErrEmptyUserID
	}

	return &WaitingEntry{ID: id, EventID: eventID, UserID: userID, JoinedAt: now}, nil
}

// Before reports whether this entry stands ahead of other. The id breaks ties,
// so two people who joined in the same instant still have a definite order.
func (e *WaitingEntry) Before(other *WaitingEntry) bool {
	if e.JoinedAt.Equal(other.JoinedAt) {
		return e.ID < other.ID
	}

	return e.JoinedAt.Before(other.JoinedAt)
}
