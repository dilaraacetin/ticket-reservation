package repository

import (
	"context"

	"ticket-reservation/internal/domain"
)

// WaitingListRepository stores the queue of users waiting for a seat of an
// event.
type WaitingListRepository interface {
	// Join puts a user at the back of the queue and returns their entry along
	// with where they now stand, counting from one. Joining twice keeps the
	// original place, so a second tap on the button cannot cost someone the time
	// they have already waited.
	//
	// The position comes back from the same operation rather than from a second
	// call, because a handoff running in between could take the caller off the
	// queue and turn a successful join into "you are not waiting".
	Join(ctx context.Context, entry *domain.WaitingEntry) (*domain.WaitingEntry, int, error)

	// Leave takes a user out of the queue. Leaving a queue one is not in is not
	// an error, because the caller wanted to be out of it either way.
	Leave(ctx context.Context, eventID, userID string) error

	// TakeNext removes the entry at the front and returns it, or
	// ErrWaitingListEmpty. Two callers racing over two freed seats have to get
	// two different people, never the same person twice.
	TakeNext(ctx context.Context, eventID string) (*domain.WaitingEntry, error)

	// Rejoin puts a taken entry back where it stood, for when the seat it was
	// taken for was gone before a hold could be made.
	Rejoin(ctx context.Context, entry *domain.WaitingEntry) error

	// Position reports where a user stands, counting from one, or ErrNotWaiting.
	Position(ctx context.Context, eventID, userID string) (int, error)

	// Drain empties an event's queue and returns who was in it. For when the
	// event is withdrawn: a queue for something that is not happening keeps
	// people waiting for a turn that will never come.
	Drain(ctx context.Context, eventID string) ([]*domain.WaitingEntry, error)
}
