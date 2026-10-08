package repository

import (
	"context"
	"time"

	"ticket-reservation/internal/domain"
)

// NotificationRepository keeps what people still have to be told.
type NotificationRepository interface {
	// Notify stores notices. One call rather than one per person, because they
	// are written in a batch when something happens to a whole event.
	Notify(ctx context.Context, notifications ...*domain.Notification) error

	// Get returns one notice. The delivery worker needs it to know what to say,
	// and it reads by id rather than carrying the words in the outbox, so the
	// wording lives in one place.
	Get(ctx context.Context, notificationID string) (*domain.Notification, error)

	// ListForUser returns a person's notices, newest first.
	ListForUser(ctx context.Context, userID string) ([]*domain.Notification, error)

	// MarkAllRead marks everything the person has not seen, and reports how many
	// that was. All of them rather than one at a time, because that is what
	// opening the list means.
	MarkAllRead(ctx context.Context, userID string, now time.Time) (int, error)
}
