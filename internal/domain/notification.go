package domain

import (
	"fmt"
	"time"
)

// NotificationKind says what happened. A closed set, like Role and SeatStatus:
// a kind nobody has written a message for would reach a reader as a blank line.
type NotificationKind string

const (
	// NotifyEventCancelled tells somebody holding a seat that the event it
	// belongs to has been withdrawn.
	NotifyEventCancelled NotificationKind = "event_cancelled"
)

// ParseNotificationKind turns a stored value back into a kind and refuses
// anything else.
func ParseNotificationKind(value string) (NotificationKind, error) {
	switch kind := NotificationKind(value); kind {
	case NotifyEventCancelled:
		return kind, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownNotificationKind, value)
	}
}

func (k NotificationKind) String() string {
	return string(k)
}

// Notification is something the service needs to tell one person, kept until
// they have seen it.
//
// Stored rather than only pushed down the live stream, because the whole point
// of being told is that you were not looking: a notice that only reaches people
// who happen to have the page open is not a notification.
type Notification struct {
	ID      string
	UserID  string
	Kind    NotificationKind
	EventID string

	// EventName is copied in rather than looked up later. The event may be gone
	// by the time anyone reads this, and "your ticket to something" is not a
	// message worth sending.
	EventName string

	// SeatID is empty when the notice is not about one seat in particular, which
	// is how somebody on the waiting list is told.
	SeatID string

	CreatedAt time.Time
	ReadAt    time.Time
}

// NewNotification returns a notice ready to be stored.
func NewNotification(
	id, userID string,
	kind NotificationKind,
	eventID, eventName, seatID string,
	now time.Time,
) (*Notification, error) {
	if id == "" {
		return nil, ErrEmptyNotificationID
	}
	if userID == "" {
		return nil, ErrEmptyUserID
	}
	if eventID == "" {
		return nil, ErrEmptyEventID
	}

	if _, err := ParseNotificationKind(kind.String()); err != nil {
		return nil, err
	}

	return &Notification{
		ID:        id,
		UserID:    userID,
		Kind:      kind,
		EventID:   eventID,
		EventName: eventName,
		SeatID:    seatID,
		CreatedAt: now,
	}, nil
}

// IsRead reports whether the person has seen it.
func (n *Notification) IsRead() bool {
	return !n.ReadAt.IsZero()
}
