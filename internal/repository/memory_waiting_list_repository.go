package repository

import (
	"context"
	"slices"
	"sync"

	"ticket-reservation/internal/domain"
)

// MemoryWaitingListRepository keeps one ordered queue per event.
type MemoryWaitingListRepository struct {
	mu     sync.Mutex
	queues map[string][]*domain.WaitingEntry
}

func NewMemoryWaitingListRepository() *MemoryWaitingListRepository {
	return &MemoryWaitingListRepository{queues: make(map[string][]*domain.WaitingEntry)}
}

// Join appends the entry unless the user already stands somewhere in the queue,
// in which case the place they already have is returned untouched.
func (r *MemoryWaitingListRepository) Join(
	_ context.Context,
	entry *domain.WaitingEntry,
) (*domain.WaitingEntry, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue := r.queues[entry.EventID]

	if index := indexOfUser(queue, entry.UserID); index >= 0 {
		existing := *queue[index]

		return &existing, index + 1, nil
	}

	stored := *entry
	queue = insertInOrder(queue, &stored)
	r.queues[entry.EventID] = queue

	joined := stored

	// Read back under the same lock, so nothing can take the caller off the
	// queue between joining it and being told where they stand.
	return &joined, indexOfUser(queue, entry.UserID) + 1, nil
}

func (r *MemoryWaitingListRepository) Leave(_ context.Context, eventID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue := r.queues[eventID]

	index := indexOfUser(queue, userID)
	if index < 0 {
		return nil
	}

	r.queues[eventID] = slices.Delete(queue, index, index+1)

	return nil
}

// TakeNext pops the front of the queue. The whole thing happens under the lock,
// so two callers cannot both walk away with the same person.
func (r *MemoryWaitingListRepository) TakeNext(
	_ context.Context,
	eventID string,
) (*domain.WaitingEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue := r.queues[eventID]
	if len(queue) == 0 {
		return nil, ErrWaitingListEmpty
	}

	taken := *queue[0]
	r.queues[eventID] = slices.Delete(queue, 0, 1)

	return &taken, nil
}

// Rejoin puts the entry back in the position its JoinedAt earns it, which is the
// one it had before it was taken.
func (r *MemoryWaitingListRepository) Rejoin(_ context.Context, entry *domain.WaitingEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue := r.queues[entry.EventID]
	if indexOfUser(queue, entry.UserID) >= 0 {
		return nil
	}

	stored := *entry
	r.queues[entry.EventID] = insertInOrder(queue, &stored)

	return nil
}

// Drain empties the queue and returns who was in it, in the order they stood.
func (r *MemoryWaitingListRepository) Drain(
	_ context.Context,
	eventID string,
) ([]*domain.WaitingEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue := r.queues[eventID]
	drained := make([]*domain.WaitingEntry, 0, len(queue))

	for _, entry := range queue {
		copied := *entry
		drained = append(drained, &copied)
	}

	delete(r.queues, eventID)

	return drained, nil
}

func (r *MemoryWaitingListRepository) Position(_ context.Context, eventID, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := indexOfUser(r.queues[eventID], userID)
	if index < 0 {
		return 0, ErrNotWaiting
	}

	return index + 1, nil
}

func indexOfUser(queue []*domain.WaitingEntry, userID string) int {
	return slices.IndexFunc(queue, func(e *domain.WaitingEntry) bool { return e.UserID == userID })
}

// insertInOrder keeps the queue sorted by JoinedAt. Appending would be enough
// for Join, but Rejoin puts back an older entry that has to land at the front.
func insertInOrder(queue []*domain.WaitingEntry, entry *domain.WaitingEntry) []*domain.WaitingEntry {
	at := len(queue)

	for i, other := range queue {
		if entry.Before(other) {
			at = i

			break
		}
	}

	return slices.Insert(queue, at, entry)
}
